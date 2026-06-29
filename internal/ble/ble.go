package ble

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/binsabbar/heartrate-monitor/internal/heartrate"
	"tinygo.org/x/bluetooth"
)

// Standard Bluetooth SIG assigned numbers for the Heart Rate service.
const (
	HeartRateServiceUUID         = 0x180D
	HeartRateMeasurementCharUUID = 0x2A37
)

// Reading re-exports the decoded heart-rate measurement type for callers.
type Reading = heartrate.Reading

// whoopName is the substring used to recognise a Whoop strap broadcasting HR.
const whoopName = "WHOOP"

var (
	adapter    = bluetooth.DefaultAdapter
	enableOnce sync.Once
	enableErr  error
)

func enableAdapter() error {
	enableOnce.Do(func() { enableErr = adapter.Enable() })
	return enableErr
}

// Device is a discovered BLE peripheral.
type Device struct {
	ID   string // platform address string (a rotating UUID on macOS)
	Name string
	RSSI int16
}

// Match selects which peripheral to connect to. ID takes precedence; if both are
// empty, the first Whoop/HR peripheral found is used.
type Match struct {
	ID   string
	Name string
}

func (m Match) matches(d Device) bool {
	if m.ID != "" {
		return strings.EqualFold(d.ID, m.ID)
	}
	if m.Name != "" {
		return strings.Contains(strings.ToUpper(d.Name), strings.ToUpper(m.Name))
	}
	return true // no constraint: accept the first candidate
}

// Scan discovers heart-rate-capable peripherals for up to d. It returns devices
// that either advertise the Heart Rate service or are named like a Whoop.
func Scan(ctx context.Context, d time.Duration) ([]Device, error) {
	if err := enableAdapter(); err != nil {
		return nil, fmt.Errorf("enable bluetooth: %w", err)
	}

	hrSvc := bluetooth.New16BitUUID(HeartRateServiceUUID)
	found := map[string]Device{}
	var mu sync.Mutex

	scanCtx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	go func() {
		<-scanCtx.Done()
		_ = adapter.StopScan()
	}()

	err := adapter.Scan(func(_ *bluetooth.Adapter, r bluetooth.ScanResult) {
		name := r.LocalName()
		isHR := r.AdvertisementPayload.HasServiceUUID(hrSvc)
		isWhoop := strings.Contains(strings.ToUpper(name), whoopName)
		if !isHR && !isWhoop {
			return
		}
		mu.Lock()
		found[r.Address.String()] = Device{ID: r.Address.String(), Name: name, RSSI: r.RSSI}
		mu.Unlock()
	})
	if err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}

	out := make([]Device, 0, len(found))
	for _, d := range found {
		out = append(out, d)
	}
	return out, nil
}

// findAddress scans until a peripheral matching m is seen, returning its live
// address (needed to connect; macOS addresses are not reconstructable from text).
func findAddress(ctx context.Context, m Match, log *slog.Logger) (bluetooth.Address, string, error) {
	hrSvc := bluetooth.New16BitUUID(HeartRateServiceUUID)
	var (
		addr  bluetooth.Address
		name  string
		found bool
		mu    sync.Mutex
	)

	scanCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		<-scanCtx.Done()
		_ = adapter.StopScan()
	}()

	err := adapter.Scan(func(_ *bluetooth.Adapter, r bluetooth.ScanResult) {
		n := r.LocalName()
		isHR := r.AdvertisementPayload.HasServiceUUID(hrSvc)
		isWhoop := strings.Contains(strings.ToUpper(n), whoopName)
		if !isHR && !isWhoop {
			return
		}
		d := Device{ID: r.Address.String(), Name: n, RSSI: r.RSSI}
		if !m.matches(d) {
			return
		}
		mu.Lock()
		addr, name, found = r.Address, n, true
		mu.Unlock()
		log.Debug("matched peripheral", "id", d.ID, "name", n, "rssi", r.RSSI)
		_ = adapter.StopScan()
	})
	if err != nil {
		return bluetooth.Address{}, "", fmt.Errorf("scan: %w", err)
	}
	if !found {
		return bluetooth.Address{}, "", context.Cause(ctx)
	}
	return addr, name, nil
}

// Monitor connects to the matching peripheral, subscribes to Heart Rate
// notifications, and forwards Readings to out until ctx is cancelled. It
// reconnects with backoff if the connection drops.
func Monitor(ctx context.Context, m Match, out chan<- Reading, log *slog.Logger) error {
	if log == nil {
		log = slog.Default()
	}
	if err := enableAdapter(); err != nil {
		return fmt.Errorf("enable bluetooth: %w", err)
	}

	backoff := time.Second
	const maxBackoff = 15 * time.Second
	for {
		if ctx.Err() != nil {
			return nil
		}
		err := connectOnce(ctx, m, out, log)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			log.Warn("connection lost, retrying", "err", err, "backoff", backoff)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		if backoff < maxBackoff {
			backoff *= 2
		}
	}
}

// connectOnce performs a single connect+subscribe cycle and blocks until the
// device disconnects or ctx is cancelled.
func connectOnce(ctx context.Context, m Match, out chan<- Reading, log *slog.Logger) error {
	addr, name, err := findAddress(ctx, m, log)
	if err != nil {
		return fmt.Errorf("find device: %w", err)
	}

	device, err := adapter.Connect(addr, bluetooth.ConnectionParams{})
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer device.Disconnect()
	log.Info("connected", "name", name, "id", addr.String())

	hrSvc := bluetooth.New16BitUUID(HeartRateServiceUUID)
	svcs, err := device.DiscoverServices([]bluetooth.UUID{hrSvc})
	if err != nil || len(svcs) == 0 {
		return fmt.Errorf("discover heart rate service: %w", err)
	}
	hrChar := bluetooth.New16BitUUID(HeartRateMeasurementCharUUID)
	chars, err := svcs[0].DiscoverCharacteristics([]bluetooth.UUID{hrChar})
	if err != nil || len(chars) == 0 {
		return fmt.Errorf("discover heart rate measurement characteristic: %w", err)
	}

	disconnected := make(chan struct{})
	var once sync.Once
	adapter.SetConnectHandler(func(_ bluetooth.Device, connected bool) {
		if !connected {
			once.Do(func() { close(disconnected) })
		}
	})

	err = chars[0].EnableNotifications(func(buf []byte) {
		r, perr := heartrate.Parse(buf)
		if perr != nil {
			log.Debug("skip unparseable HR payload", "err", perr, "len", len(buf))
			return
		}
		select {
		case out <- r:
		case <-ctx.Done():
		}
	})
	if err != nil {
		return fmt.Errorf("enable notifications: %w", err)
	}

	select {
	case <-ctx.Done():
		return nil
	case <-disconnected:
		return fmt.Errorf("device disconnected")
	}
}
