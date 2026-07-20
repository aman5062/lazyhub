package tui

import (
	"context"
	"fmt"
	"testing"
)

func TestLoginPreview(t *testing.T) {
	m := newLoginModel(context.Background())
	m.width = 90
	fmt.Println("\n===== CHOOSE (cursor on PAT) =====")
	fmt.Println(m.View())
	m.cursor = 1
	fmt.Println("\n===== CHOOSE (cursor on device) =====")
	fmt.Println(m.View())
	m.state = stToken
	m.input.Focus()
	fmt.Println("\n===== TOKEN ENTRY =====")
	fmt.Println(m.View())
}
