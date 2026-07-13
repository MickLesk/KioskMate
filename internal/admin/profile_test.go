package admin

import (
	"testing"

	"github.com/MickLesk/KioskMate/internal/hardware"
)

func TestPerformanceRecommendationForRaspberryPi4(t *testing.T) {
	result := performanceRecommendation(hardware.Status{
		Device:  map[string]any{"model": "Raspberry Pi 4 Model B Rev 1.1"},
		System:  map[string]any{"memory_size_gib": 3.3},
		Session: map[string]string{"type": "wayland"},
	})
	if result["profile"] != "raspberry" || result["gpu_mode"] != "auto" {
		t.Fatalf("recommendation = %#v", result)
	}
}

func TestPerformanceRecommendationForRaspberryPi5(t *testing.T) {
	result := performanceRecommendation(hardware.Status{
		Device:  map[string]any{"model": "Raspberry Pi 5 Model B"},
		System:  map[string]any{"memory_size_gib": 8.0},
		Session: map[string]string{"type": "wayland"},
	})
	if result["profile"] != "balanced" || result["gpu_mode"] != "hardware" {
		t.Fatalf("recommendation = %#v", result)
	}
}
