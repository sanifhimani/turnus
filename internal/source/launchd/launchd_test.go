package launchd

import (
	"context"
	"testing"
	"time"
)

func BenchmarkDiscover(b *testing.B) {
	for _, builtin := range []bool{false, true} {
		name := "thirdparty"
		if builtin {
			name = "builtin"
		}
		b.Run(name, func(b *testing.B) {
			for b.Loop() {
				if _, err := Discover(context.Background(), time.Now(), builtin); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
