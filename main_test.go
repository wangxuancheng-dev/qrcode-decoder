package main

import (
	"context"
	"strings"
	"testing"
)

func TestDecodeProblemURL(t *testing.T) {
	url := "https://0625d9d5e411e970.sousd.com/t/2606061133562620536.png"
	img, err := loadImageFromURL(context.Background(), url)
	if err != nil {
		t.Fatalf("load image: %v", err)
	}
	content, err := decodeQRCode(img)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if content == "" {
		t.Fatal("empty content")
	}
	if !strings.HasPrefix(content, "000201") && !strings.HasPrefix(content, "http") {
		t.Fatalf("unexpected content: %s", content)
	}
}
