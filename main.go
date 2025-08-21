package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"unsafe"

	evdev "github.com/gvalkov/golang-evdev"
)

var trackballKeywords = []string{
	"trackball",
	"expert mouse",
	"orbit",
	"slimblade",
}

const (
	DEFAULT_SENSITIVITY = 0.3
	DEFAULT_DEAD_ZONE   = 2
	MAX_EVENT_DEVICES   = 32
	DEVICE_SETUP_DELAY  = 100 * time.Millisecond
)

// Linux uinput constants for virtual input device creation
const (
	UINPUT_MAX_NAME_SIZE = 80
	UI_SET_EVBIT         = 0x40045564
	UI_SET_RELBIT        = 0x40045566
	UI_DEV_SETUP         = 0x405c5503
	UI_DEV_CREATE        = 0x5501
	UI_DEV_DESTROY       = 0x5502
	EV_REL               = 0x02
	REL_WHEEL            = 0x08
	REL_HWHEEL           = 0x06
	EV_SYN               = 0x00
	SYN_REPORT           = 0x00
)

// UinputSetup defines the virtual device configuration for uinput interface
type UinputSetup struct {
	ID   InputID
	Name [UINPUT_MAX_NAME_SIZE]byte
	_    uint32 // ff_effects_max (unused)
}

// InputID contains device identification information
type InputID struct {
	Bustype uint16
	Vendor  uint16
	Product uint16
	Version uint16
}

// InputEvent represents a Linux input event structure
type InputEvent struct {
	Time  syscall.Timeval
	Type  uint16
	Code  uint16
	Value int32
}

// TrackballScroller manages trackball input conversion to scroll events
type TrackballScroller struct {
	device      *evdev.InputDevice
	virtualFd   int
	sensitivity float64
	deadZone    int32
}

// findTrackballDevices searches for connected trackball devices
func findTrackballDevices() ([]string, error) {
	var trackballPaths []string

	for i := 0; i < MAX_EVENT_DEVICES; i++ {
		devicePath := fmt.Sprintf("/dev/input/event%d", i)

		device, err := evdev.Open(devicePath)
		if err != nil {
			continue
		}

		if isTrackballDevice(device.Name) {
			trackballPaths = append(trackballPaths, devicePath)
			fmt.Printf("Found trackball: %s (%s)\n", device.Name, devicePath)
		}

		device.File.Close()
	}

	return trackballPaths, nil
}

// isTrackballDevice checks if a device name matches known trackball patterns
func isTrackballDevice(deviceName string) bool {
	name := strings.ToLower(deviceName)
	for _, keyword := range trackballKeywords {
		if strings.Contains(name, keyword) {
			return true
		}
	}
	return false
}

// openTrackballDevice grabs the specified input device
func openTrackballDevice(devicePath string) (*evdev.InputDevice, error) {
	device, err := evdev.Open(devicePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open device %s: %w", devicePath, err)
	}

	if err := device.Grab(); err != nil {
		return nil, fmt.Errorf("failed to grab device %s: %w", devicePath, err)
	}

	return device, nil
}

// createScrollOnlyDevice creates a virtual uinput device for scroll events
func createScrollOnlyDevice() (int, error) {
	fd, err := syscall.Open("/dev/uinput", syscall.O_WRONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return -1, fmt.Errorf("failed to open /dev/uinput: %w", err)
	}

	if err := configureDevice(fd); err != nil {
		syscall.Close(fd)
		return -1, err
	}

	if err := setupDevice(fd); err != nil {
		syscall.Close(fd)
		return -1, err
	}

	if err := createDevice(fd); err != nil {
		syscall.Close(fd)
		return -1, err
	}

	time.Sleep(DEVICE_SETUP_DELAY)
	return fd, nil
}

func configureDevice(fd int) error {
	capabilities := []struct {
		cmd   uintptr
		value uintptr
		name  string
	}{
		{UI_SET_EVBIT, EV_REL, "EV_REL"},
		{UI_SET_RELBIT, REL_WHEEL, "REL_WHEEL"},
		{UI_SET_RELBIT, REL_HWHEEL, "REL_HWHEEL"},
		{UI_SET_EVBIT, EV_SYN, "EV_SYN"},
	}

	for _, cap := range capabilities {
		if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), cap.cmd, cap.value); errno != 0 {
			return fmt.Errorf("failed to set %s: %v", cap.name, errno)
		}
	}

	return nil
}

func setupDevice(fd int) error {
	var setup UinputSetup
	copy(setup.Name[:], "Trackball Scroll Device")
	setup.ID.Bustype = 0x03 // USB
	setup.ID.Vendor = 0x1234
	setup.ID.Product = 0x5678
	setup.ID.Version = 1

	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), UI_DEV_SETUP, uintptr(unsafe.Pointer(&setup))); errno != 0 {
		return fmt.Errorf("failed to setup device: %v", errno)
	}

	return nil
}

func createDevice(fd int) error {
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), UI_DEV_CREATE, 0); errno != 0 {
		return fmt.Errorf("failed to create device: %v", errno)
	}
	return nil
}

func newTrackballScroller(device *evdev.InputDevice, sensitivity float64, deadZone int32) (*TrackballScroller, error) {
	virtualFd, err := createScrollOnlyDevice()
	if err != nil {
		return nil, fmt.Errorf("cannot create virtual device: %w", err)
	}

	return &TrackballScroller{
		device:      device,
		virtualFd:   virtualFd,
		sensitivity: sensitivity,
		deadZone:    deadZone,
	}, nil
}

func (ts *TrackballScroller) sendScrollEvent(isHorizontal bool, value int32) error {
	code := uint16(REL_WHEEL)
	if isHorizontal {
		code = uint16(REL_HWHEEL)
	}

	now := time.Now()
	events := []InputEvent{
		{
			Time:  syscall.Timeval{Sec: now.Unix(), Usec: 0},
			Type:  uint16(EV_REL),
			Code:  code,
			Value: value,
		},
		{
			Time:  syscall.Timeval{Sec: now.Unix(), Usec: 0},
			Type:  uint16(EV_SYN),
			Code:  uint16(SYN_REPORT),
			Value: 0,
		},
	}

	for _, event := range events {
		eventBytes := (*(*[unsafe.Sizeof(event)]byte)(unsafe.Pointer(&event)))[:]
		if _, err := syscall.Write(ts.virtualFd, eventBytes); err != nil {
			return fmt.Errorf("failed to write event: %w", err)
		}
	}

	return nil
}

func (ts *TrackballScroller) close() {
	if ts.virtualFd >= 0 {
		syscall.Syscall(syscall.SYS_IOCTL, uintptr(ts.virtualFd), UI_DEV_DESTROY, 0)
		syscall.Close(ts.virtualFd)
	}

	if ts.device != nil {
		ts.device.Release()
	}
}

func (ts *TrackballScroller) processEvents(stopChan <-chan struct{}) error {
	for {
		select {
		case <-stopChan:
			return nil
		default:
		}

		events, err := ts.device.Read()
		if err != nil {
			return fmt.Errorf("error reading events: %w", err)
		}

		ts.handleEvents(events)
	}
}

func (ts *TrackballScroller) handleEvents(events []evdev.InputEvent) {
	for _, event := range events {
		if event.Type != evdev.EV_REL {
			continue
		}

		var isHorizontal bool
		var scrollValue int32

		switch event.Code {
		case evdev.REL_X:
			isHorizontal = true
			scrollValue = int32(float64(event.Value) * ts.sensitivity)
		case evdev.REL_Y:
			isHorizontal = false
			scrollValue = -int32(float64(event.Value) * ts.sensitivity) // Inverted for natural scrolling
		default:
			continue
		}

		if abs(event.Value) > ts.deadZone && scrollValue != 0 {
			ts.sendScrollEvent(isHorizontal, scrollValue)
		}
	}
}

func abs(x int32) int32 {
	if x < 0 {
		return -x
	}
	return x
}

func selectDevice(devicePath string) (string, error) {
	if devicePath != "auto" {
		return devicePath, nil
	}

	fmt.Println("Detecting trackball devices...")
	trackballs, err := findTrackballDevices()
	if err != nil {
		return "", fmt.Errorf("failed to scan for devices: %w", err)
	}

	if len(trackballs) == 0 {
		return "", fmt.Errorf("no trackball devices found. Try to manually add a device with -device")
	}

	if len(trackballs) > 1 {
		fmt.Println("Multiple trackballs found:")
		for i, path := range trackballs {
			fmt.Printf("  %d: %s\n", i+1, path)
		}
		fmt.Printf("Using first one: %s\n", trackballs[0])
	}

	return trackballs[0], nil
}

func setupSignalHandling() <-chan struct{} {
	stopChan := make(chan struct{})
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-signalChan
		close(stopChan)
	}()

	return stopChan
}

func main() {
	// Parse command line arguments
	sensitivity := flag.Float64("sensitivity", DEFAULT_SENSITIVITY, "Scroll sensitivity")
	deadZone := flag.Int("deadzone", DEFAULT_DEAD_ZONE, "Dead zone for ignoring small movements")
	devicePath := flag.String("device", "auto", "Path to find trackball device")
	flag.Parse()

	fmt.Println("Trackball Scroll - Converting trackball movement to scroll events")

	// Determine target device
	finalDevicePath, err := selectDevice(*devicePath)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Device: %s | Sensitivity: %.2f | Dead zone: %d\n", finalDevicePath, *sensitivity, *deadZone)

	// Open and configure trackball device
	device, err := openTrackballDevice(finalDevicePath)
	if err != nil {
		log.Fatalf("Failed to open device: %v", err)
	}

	// Create scroller instance
	scroller, err := newTrackballScroller(device, *sensitivity, int32(*deadZone))
	if err != nil {
		log.Fatalf("Failed to create scroller: %v", err)
	}
	defer scroller.close()

	fmt.Printf("Ready: %s | Press Ctrl+C to exit\n", device.Name)

	// Setup graceful shutdown and start processing
	stopChan := setupSignalHandling()
	if err := scroller.processEvents(stopChan); err != nil {
		log.Fatalf("Error processing events: %v", err)
	}

	fmt.Println("Trackball scroller stopped.")
}
