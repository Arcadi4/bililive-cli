package cmd

import (
	"fmt"
	"io"

	"github.com/skip2/go-qrcode"
)

func renderLoginQRCode(w io.Writer, value string) error {
	code, err := qrcode.New(value, qrcode.Medium)
	if err != nil {
		return fmt.Errorf("encode QR code: %w", err)
	}
	bitmap := code.Bitmap()
	for y := 0; y < len(bitmap); y += 2 {
		for x := range bitmap[y] {
			top := bitmap[y][x]
			bottom := y+1 < len(bitmap) && bitmap[y+1][x]
			switch {
			case top && bottom:
				fmt.Fprint(w, "█")
			case top:
				fmt.Fprint(w, "▀")
			case bottom:
				fmt.Fprint(w, "▄")
			default:
				fmt.Fprint(w, " ")
			}
		}
		fmt.Fprintln(w)
	}
	return nil
}
