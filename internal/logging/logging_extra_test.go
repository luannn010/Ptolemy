package logging

import (
	"testing"

	"github.com/rs/zerolog"
)

func TestSetup_LevelMapping(t *testing.T) {
	cases := []struct {
		in   string
		want zerolog.Level
	}{
		{"panic", zerolog.PanicLevel},
		{"fatal", zerolog.FatalLevel},
		{"error", zerolog.ErrorLevel},
		{"warn", zerolog.WarnLevel},
		{"info", zerolog.InfoLevel},
		{"debug", zerolog.DebugLevel},
		{"trace", zerolog.TraceLevel},
		{"", zerolog.DebugLevel},
		{"BoGuS", zerolog.DebugLevel},
		{"INFO", zerolog.InfoLevel},
	}
	for _, tc := range cases {
		Setup(tc.in)
		if got := zerolog.GlobalLevel(); got != tc.want {
			t.Fatalf("Setup(%q): got %v want %v", tc.in, got, tc.want)
		}
	}
}
