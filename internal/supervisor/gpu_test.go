package supervisor

import (
	"encoding/json"
	"testing"
)

func TestGPUInfoDetectsSoftwareRaster(t *testing.T) {
	var result systemInfoResult
	raw := `{"gpu":{"devices":[{"vendorString":"Broadcom","deviceString":"V3D 4.2","driverVendor":"Mesa","driverVersion":"23.2"}],"featureStatus":{"rasterization":"disabled_software","gpu_compositing":"enabled","video_decode":"unavailable_off"}}}`
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatal(err)
	}
	info := result.gpuInfo()
	if !info.SoftwareRaster || info.Renderer != "V3D 4.2" || info.Driver != "Mesa 23.2" {
		t.Fatalf("gpu info = %#v", info)
	}
	if len(info.SoftwareFeature) != 2 || info.SoftwareFeature[0] != "rasterization" {
		t.Fatalf("software features = %#v", info.SoftwareFeature)
	}
}
