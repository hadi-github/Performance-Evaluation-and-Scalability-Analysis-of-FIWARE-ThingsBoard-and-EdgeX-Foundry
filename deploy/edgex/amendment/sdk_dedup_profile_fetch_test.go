// Amended-SDK behavioural test: one profile fetch per DISTINCT profile.
//
// Compiled and run inside the amended image's build (see
// deploy/edgex/amendment/Dockerfile.device-rest). It is the gate that proves the
// dedup patch is actually in effect: a successful `go build` cannot prove that,
// because upstream builds with -mod=readonly, which silently ignores vendor/.
//
// The test is offline and needs no cluster. It drives the real InitCache from
// device-sdk-go v4.0.2 and counts DeviceProfileByName round-trips.
//
// Against unpatched v4.0.2 this fails with:
//
//	50 devices sharing one profile caused 50 fetches; want 1
//
// which is the measured O(devices) behaviour described in
// research/findings/edgex-ordinal-15-readiness-failure.md.
package cache

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/edgexfoundry/go-mod-core-contracts/v4/clients/interfaces"
	"github.com/edgexfoundry/go-mod-core-contracts/v4/clients/logger"
	"github.com/edgexfoundry/go-mod-core-contracts/v4/dtos"
	"github.com/edgexfoundry/go-mod-core-contracts/v4/dtos/responses"
	"github.com/edgexfoundry/go-mod-core-contracts/v4/errors"
	gometrics "github.com/rcrowley/go-metrics"

	bootstrapContainer "github.com/edgexfoundry/go-mod-bootstrap/v4/bootstrap/container"
	"github.com/edgexfoundry/go-mod-bootstrap/v4/di"
)

// countingProfileClient records how many DeviceProfileByName round-trips happen.
type countingProfileClient struct {
	interfaces.DeviceProfileClient
	calls map[string]int
}

func (c *countingProfileClient) DeviceProfileByName(ctx context.Context, name string) (responses.DeviceProfileResponse, errors.EdgeX) {
	c.calls[name]++
	return responses.DeviceProfileResponse{
		Profile: dtos.DeviceProfile{DeviceProfileBasicInfo: dtos.DeviceProfileBasicInfo{Name: name}},
	}, nil
}

type countingDeviceClient struct {
	interfaces.DeviceClient
	devices []dtos.Device
}

func (c *countingDeviceClient) DevicesByServiceName(ctx context.Context, name string, offset, limit int) (responses.MultiDevicesResponse, errors.EdgeX) {
	return responses.MultiDevicesResponse{Devices: c.devices}, nil
}

type emptyPWClient struct {
	interfaces.ProvisionWatcherClient
}

func (e *emptyPWClient) ProvisionWatchersByServiceName(ctx context.Context, name string, offset, limit int) (responses.MultiProvisionWatchersResponse, errors.EdgeX) {
	return responses.MultiProvisionWatchersResponse{}, nil
}

// stubMetrics satisfies interfaces.MetricsManager; the cache only calls Register.
type stubMetrics struct{}

func (stubMetrics) ResetInterval(time.Duration)                          {}
func (stubMetrics) Register(string, interface{}, map[string]string) error { return nil }
func (stubMetrics) IsRegistered(string) bool                             { return false }
func (stubMetrics) Unregister(string)                                    {}
func (stubMetrics) Run(context.Context, *sync.WaitGroup)                 {}
func (stubMetrics) GetCounter(string) gometrics.Counter                  { return nil }
func (stubMetrics) GetGauge(string) gometrics.Gauge                      { return nil }
func (stubMetrics) GetGaugeFloat64(string) gometrics.GaugeFloat64        { return nil }
func (stubMetrics) GetTimer(string) gometrics.Timer                      { return nil }

func TestInitCacheDedupProfileFetch(t *testing.T) {
	// All devices share one profile, exactly as the benchmark provisions them.
	const devices = 50
	devs := make([]dtos.Device, devices)
	for i := range devs {
		devs[i] = dtos.Device{Name: "bench-x-000", ProfileName: "bench-gen-v1", ServiceName: "device-rest"}
	}
	dpc := &countingProfileClient{calls: map[string]int{}}
	dic := di.NewContainer(di.ServiceConstructorMap{
		bootstrapContainer.DeviceClientName: func(get di.Get) any { return &countingDeviceClient{devices: devs} },
		bootstrapContainer.DeviceProfileClientName: func(get di.Get) any { return dpc },
		bootstrapContainer.ProvisionWatcherClientName: func(get di.Get) any { return &emptyPWClient{} },
		bootstrapContainer.MetricsManagerInterfaceName: func(get di.Get) any { return stubMetrics{} },
		bootstrapContainer.LoggingClientInterfaceName: func(get di.Get) any { return logger.NewMockClient() },
	})
	if err := InitCache("device-rest", "device-rest", dic); err != nil {
		t.Fatalf("InitCache: %v", err)
	}
	if got := dpc.calls["bench-gen-v1"]; got != 1 {
		t.Fatalf("%d devices sharing one profile caused %d fetches; want 1", devices, got)
	}
	if got := len(pc.All()); got != 1 {
		t.Fatalf("profileCache holds %d profiles; want 1", got)
	}
}
