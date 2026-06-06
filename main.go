package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
)

const maxImageSize = 10 << 20 // 10MB

type Request struct {
	Base64 string `json:"base64" form:"base64"`
	URL    string `json:"url" form:"url"`
}

type Response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Content string `json:"content,omitempty"`
}

func decodeHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	if r.Method != http.MethodPost {
		json.NewEncoder(w).Encode(Response{Code: 1, Message: "仅支持POST请求"})
		return
	}

	var req Request
	contentType := r.Header.Get("Content-Type")

	if strings.Contains(contentType, "application/json") {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			json.NewEncoder(w).Encode(Response{Code: 1, Message: "JSON格式错误"})
			return
		}
	} else {
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			if err := r.ParseForm(); err != nil {
				json.NewEncoder(w).Encode(Response{Code: 1, Message: "参数解析失败"})
				return
			}
		}
		req.Base64 = r.PostForm.Get("base64")
		req.URL = r.PostForm.Get("url")
	}

	img, err := loadImage(r.Context(), strings.TrimSpace(req.URL), strings.TrimSpace(req.Base64))
	if err != nil {
		json.NewEncoder(w).Encode(Response{Code: 1, Message: err.Error()})
		return
	}

	content, err := decodeQRCode(img)
	if err != nil {
		json.NewEncoder(w).Encode(Response{Code: 1, Message: err.Error()})
		return
	}

	json.NewEncoder(w).Encode(Response{
		Code:    0,
		Message: "success",
		Content: content,
	})
}

func loadImage(ctx context.Context, imageURL, b64Raw string) (image.Image, error) {
	if imageURL != "" {
		return loadImageFromURL(ctx, imageURL)
	}
	if b64Raw != "" {
		return loadImageFromBase64(b64Raw)
	}
	return nil, fmt.Errorf("base64或url不能为空")
}

func loadImageFromBase64(b64Raw string) (image.Image, error) {
	b64data := b64Raw
	if idx := strings.Index(b64data, ","); idx != -1 {
		b64data = b64data[idx+1:]
	}

	imgBytes, err := base64.StdEncoding.DecodeString(b64data)
	if err != nil {
		return nil, fmt.Errorf("base64解码失败")
	}

	img, _, err := image.Decode(bytes.NewReader(imgBytes))
	if err != nil {
		return nil, fmt.Errorf("图片格式不支持")
	}
	return img, nil
}

func loadImageFromURL(ctx context.Context, rawURL string) (image.Image, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("图片地址无效")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("图片地址仅支持http或https")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("图片地址无效")
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; qrcode-decoder/1.0)")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("图片下载失败")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("图片下载失败")
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxImageSize+1))
	if err != nil {
		return nil, fmt.Errorf("图片下载失败")
	}
	if len(body) > maxImageSize {
		return nil, fmt.Errorf("图片过大")
	}

	img, _, err := image.Decode(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("图片格式不支持")
	}
	return img, nil
}

func decodeQRCode(img image.Image) (string, error) {
	tryHarder := map[gozxing.DecodeHintType]interface{}{
		gozxing.DecodeHintType_TRY_HARDER: true,
	}
	pureBarcode := map[gozxing.DecodeHintType]interface{}{
		gozxing.DecodeHintType_PURE_BARCODE: true,
		gozxing.DecodeHintType_TRY_HARDER:   true,
	}

	// 截图场景为主：优先纯条码模式，标准定位作拍照图兜底
	attempts := []struct {
		img   image.Image
		hints map[gozxing.DecodeHintType]interface{}
		binar func(gozxing.LuminanceSource) gozxing.Binarizer
	}{
		{img, pureBarcode, gozxing.NewHybridBinarizer},
		{img, pureBarcode, gozxing.NewGlobalHistgramBinarizer},
		{scaleImageNearest(img, 3), pureBarcode, gozxing.NewHybridBinarizer},
		{scaleImageNearest(img, 4), pureBarcode, gozxing.NewHybridBinarizer},
		{img, tryHarder, gozxing.NewHybridBinarizer},
	}

	reader := qrcode.NewQRCodeReader()
	for _, attempt := range attempts {
		bmp, err := newBinaryBitmap(attempt.img, attempt.binar)
		if err != nil {
			continue
		}
		result, err := reader.Decode(bmp, attempt.hints)
		if err == nil && result.GetText() != "" {
			return result.GetText(), nil
		}
	}
	return "", fmt.Errorf("二维码识别失败")
}

func newBinaryBitmap(img image.Image, binar func(gozxing.LuminanceSource) gozxing.Binarizer) (*gozxing.BinaryBitmap, error) {
	src := gozxing.NewLuminanceSourceFromImage(img)
	return gozxing.NewBinaryBitmap(binar(src))
}

func scaleImageNearest(img image.Image, scale int) image.Image {
	if scale <= 1 {
		return img
	}
	bounds := img.Bounds()
	w, h := bounds.Dx()*scale, bounds.Dy()*scale
	scaled := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			scaled.Set(x, y, img.At(bounds.Min.X+x/scale, bounds.Min.Y+y/scale))
		}
	}
	return scaled
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
		"name":   "qrcode-decoder",
		"engine": "gozxing",
	})
}

func getPort() string {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8086"
	}
	return ":" + port
}

func main() {
	_ = godotenv.Load()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /decode", decodeHandler)
	mux.HandleFunc("GET /health", healthHandler)

	server := &http.Server{
		Addr:    getPort(),
		Handler: mux,
	}

	// 优雅退出监听
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("✅ 服务启动成功 | 监听端口: %s\n", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("❌ 服务启动失败: %v", err)
		}
	}()

	<-quit
	log.Println("🛑 正在关闭服务，优雅退出中...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("❌ 服务关闭异常: %v", err)
	}

	log.Println("✅ 服务已安全关闭")
}
