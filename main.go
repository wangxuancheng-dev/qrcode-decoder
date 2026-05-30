package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"log"
	"net/http"
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

type Request struct {
	Base64 string `json:"base64" form:"base64"`
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
	}

	b64Raw := strings.TrimSpace(req.Base64)
	if b64Raw == "" {
		json.NewEncoder(w).Encode(Response{Code: 1, Message: "base64不能为空"})
		return
	}

	// 剔除 data:image 前缀
	b64data := b64Raw
	if idx := strings.Index(b64data, ","); idx != -1 {
		b64data = b64data[idx+1:]
	}

	// Base64解码
	imgBytes, err := base64.StdEncoding.DecodeString(b64data)
	if err != nil {
		json.NewEncoder(w).Encode(Response{Code: 1, Message: "base64解码失败"})
		return
	}

	// 图片解码
	img, _, err := image.Decode(bytes.NewReader(imgBytes))
	if err != nil {
		json.NewEncoder(w).Encode(Response{Code: 1, Message: "图片格式不支持"})
		return
	}

	// 二维码识别
	bmp, err := gozxing.NewBinaryBitmapFromImage(img)
	if err != nil {
		json.NewEncoder(w).Encode(Response{Code: 1, Message: "图片处理失败"})
		return
	}

	reader := qrcode.NewQRCodeReader()
	result, err := reader.Decode(bmp, nil)
	if err != nil {
		json.NewEncoder(w).Encode(Response{Code: 1, Message: "二维码识别失败"})
		return
	}

	json.NewEncoder(w).Encode(Response{
		Code:    0,
		Message: "success",
		Content: result.GetText(),
	})
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