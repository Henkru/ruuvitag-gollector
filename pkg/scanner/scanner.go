package scanner

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/niktheblak/ruuvitag-gollector/pkg/config"
	"github.com/niktheblak/ruuvitag-gollector/pkg/exporter"
	"github.com/niktheblak/ruuvitag-gollector/pkg/sensor"
	"github.com/paypal/gatt"
)

type Scanner struct {
	SleepInterval        time.Duration
	Exporters            []exporter.Exporter
	quit                 chan int
	stopScan             chan int
	measurements         chan sensor.Data
	deviceIDs            []gatt.UUID
	deviceNames          map[string]string
	deviceCreator        deviceCreator
	peripheralDiscoverer peripheralDiscoverer
}

func New(cfg config.Config) (*Scanner, error) {
	scn := &Scanner{
		SleepInterval:        cfg.ReportingInterval.Duration,
		quit:                 make(chan int),
		stopScan:             make(chan int),
		measurements:         make(chan sensor.Data),
		deviceNames:          make(map[string]string),
		deviceCreator:        gattDeviceCreator{},
		peripheralDiscoverer: gattPeripheralDiscoverer{},
	}
	for _, rt := range cfg.RuuviTags {
		uid, err := gatt.ParseUUID(rt.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to parse RuuviTag UUID %s: %w", rt.ID, err)
		}
		scn.deviceIDs = append(scn.deviceIDs, uid)
		scn.deviceNames[rt.ID] = rt.Name
	}
	if len(scn.deviceIDs) > 0 {
		log.Printf("Reading from RuuviTags %v", scn.deviceIDs)
	} else {
		log.Println("Reading from all nearby BLE devices")
	}
	return scn, nil
}

func (s *Scanner) Start(ctx context.Context) error {
	device, err := s.deviceCreator.NewDevice()
	if err != nil {
		return fmt.Errorf("failed to open device: %w", err)
	}
	s.peripheralDiscoverer.HandlePeripheralDiscovered(device, s.onPeripheralDiscovered)
	if err := device.Init(s.onStateChanged); err != nil {
		return fmt.Errorf("failed to initialize device: %w", err)
	}
	go s.exportMeasurements(ctx)
	return nil
}

func (s *Scanner) Stop() {
	s.quit <- 1
}

func (s *Scanner) beginScan(d gatt.Device) {
	log.Println("Scanner starting")
	log.Printf("Scanner scanning devices %v", s.deviceIDs)
	d.Scan(s.deviceIDs, false)
	timer := time.NewTimer(s.SleepInterval)
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
			log.Printf("Scanner scanning devices %v", s.deviceIDs)
			d.Scan(s.deviceIDs, false)
		case <-s.stopScan:
			log.Println("Scanner stopping")
			return
		case <-s.quit:
			log.Println("Scanner quitting")
			return
		}
	}
}

func (s *Scanner) onStateChanged(d gatt.Device, state gatt.State) {
	switch state {
	case gatt.StatePoweredOn:
		log.Println("Device powered on")
		go s.beginScan(d)
	case gatt.StatePoweredOff:
		log.Println("Device powered off")
		s.stopScan <- 1
		// Attempt to restart device
		if err := d.Init(s.onStateChanged); err != nil {
			log.Printf("Failed to restart device: %v", err)
		}
	default:
		log.Printf("Unhandled state: %v", state)
	}
}

func (s *Scanner) onPeripheralDiscovered(p gatt.Peripheral, a *gatt.Advertisement, rssi int) {
	log.Printf("Read sensor data from device %s:%s", p.ID(), p.Name())
	data, err := sensor.Parse(a.ManufacturerData)
	if err != nil {
		var header []byte
		if len(a.ManufacturerData) >= 3 {
			header = a.ManufacturerData[:3]
		} else {
			header = a.ManufacturerData
		}
		log.Printf("Error while parsing RuuviTag data (%d bytes) %v: %v", len(a.ManufacturerData), header, err)
		return
	}
	data.DeviceID = p.ID()
	data.Name = s.deviceNames[p.ID()]
	data.Timestamp = time.Now()
	s.measurements <- data
}

func (s *Scanner) exportMeasurements(ctx context.Context) {
	for {
		select {
		case m := <-s.measurements:
			log.Printf("Received measurement from sensor %v", m.Name)
			for _, e := range s.Exporters {
				log.Printf("Exporting measurement to %v", e.Name())
				if err := e.Export(ctx, m); err != nil {
					log.Printf("Failed to report measurement: %v", err)
				}
			}
		case <-s.quit:
			return
		}
	}
}
