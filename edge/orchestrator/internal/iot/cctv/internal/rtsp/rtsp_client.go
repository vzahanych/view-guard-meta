package rtsp

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v4"
	"github.com/bluenviron/gortsplib/v4/pkg/base"
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtph264"
	"github.com/pion/rtp"
	eventbus "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus"
	evtbusstypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus/types"
	"go.uber.org/zap"
)

// RTSPClient manages RTSP stream connections
type RTSPClient struct {
	logger       *zap.Logger
	eventBus     eventbus.EventBus // Optional: for publishing events
	url          string
	username     string
	password     string
	client       *gortsplib.Client
	connected    bool
	reconnecting bool
	mu           sync.RWMutex
	ctx          context.Context
	cancel       context.CancelFunc
	onFrame      func([]byte, time.Time) // Callback for received frames
	lastFrame    time.Time
	healthStatus string
}

// RTSPClientConfig contains RTSP client configuration
type RTSPClientConfig struct {
	URL               string
	Username          string
	Password          string
	Timeout           time.Duration
	ReconnectInterval time.Duration
	OnFrameCallback   func([]byte, time.Time)
}

// NewRTSPClient creates a new RTSP client
func NewRTSPClient(config RTSPClientConfig, log *zap.Logger) *RTSPClient {
	ctx, cancel := context.WithCancel(context.Background())

	client := &RTSPClient{
		logger:       log,
		url:          config.URL,
		username:     config.Username,
		password:     config.Password,
		ctx:          ctx,
		cancel:       cancel,
		onFrame:      config.OnFrameCallback,
		healthStatus: "disconnected",
	}

	return client
}

// SetEventBus sets the event bus for publishing events
func (c *RTSPClient) SetEventBus(bus eventbus.EventBus) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.eventBus = bus
}

// Name returns the service name
func (c *RTSPClient) Name() string {
	return fmt.Sprintf("rtsp-client-%s", c.url)
}

// Start starts the RTSP client
func (c *RTSPClient) Start(ctx context.Context) error {
	c.logger.Info("Starting RTSP client", zap.String("url", c.url))

	// Start connection goroutine
	go c.run()

	return nil
}

// Stop stops the RTSP client
func (c *RTSPClient) Stop(ctx context.Context) error {
	c.logger.Info("Stopping RTSP client", zap.String("url", c.url))

	c.cancel()

	// Close connection if open
	c.mu.Lock()
	if c.client != nil {
		c.client.Close()
		c.client = nil
	}
	c.connected = false
	c.mu.Unlock()

	return nil
}

// run manages the connection lifecycle
func (c *RTSPClient) run() {
	for {
		select {
		case <-c.ctx.Done():
			return
		default:
			if err := c.connect(); err != nil {
				c.logger.Error("RTSP connection failed", zap.Error(err), zap.String("url", c.url))
				c.mu.Lock()
				c.connected = false
				c.healthStatus = "error"
				c.mu.Unlock()

				// Publish disconnection event
				if c.eventBus != nil {
					c.eventBus.Publish(evtbusstypes.Event{
						Type:      evtbusstypes.EventTypeDeviceDisconnected,
						Source:    c.Name(),
						Timestamp: time.Now(),
						Data: map[string]interface{}{
							"url":    c.url,
							"reason": err.Error(),
						},
					})
				}

				// Wait before reconnecting
				select {
				case <-c.ctx.Done():
					return
				case <-time.After(5 * time.Second): // Reconnect interval
				}
				continue
			}

			// Connection successful, monitor health
			c.monitorHealth()
		}
	}
}

// connect establishes RTSP connection
func (c *RTSPClient) connect() error {
	c.mu.Lock()
	if c.reconnecting {
		c.mu.Unlock()
		return fmt.Errorf("already reconnecting")
	}
	c.reconnecting = true
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		c.reconnecting = false
		c.mu.Unlock()
	}()

	c.logger.Info("Connecting to RTSP stream", zap.String("url", c.url))

	// Parse URL
	u, err := base.ParseURL(c.url)
	if err != nil {
		return fmt.Errorf("failed to parse URL: %w", err)
	}

	// Add credentials to URL if provided and not already present
	if c.username != "" && c.password != "" && u.User == nil {
		u.User = url.UserPassword(c.username, c.password)
	}

	// Create client
	client := &gortsplib.Client{}

	// Connect to server
	desc, _, err := client.Describe(u)
	if err != nil {
		return fmt.Errorf("failed to describe stream: %w", err)
	}

	// Find H.264 format
	var h264Format *format.H264
	var h264Media *description.Media
	for _, media := range desc.Medias {
		for _, forma := range media.Formats {
			if h264, ok := forma.(*format.H264); ok {
				h264Format = h264
				h264Media = media
				break
			}
		}
		if h264Format != nil {
			break
		}
	}

	if h264Format == nil {
		return fmt.Errorf("H.264 format not found in stream")
	}

	// Setup stream
	err = client.SetupAll(desc.BaseURL, desc.Medias)
	if err != nil {
		return fmt.Errorf("failed to setup stream: %w", err)
	}

	// Create H.264 decoder
	h264Decoder := &rtph264.Decoder{}
	err = h264Decoder.Init()
	if err != nil {
		return fmt.Errorf("failed to init decoder: %w", err)
	}

	// Setup packet handler
	client.OnPacketRTP(h264Media, h264Format, func(pkt *rtp.Packet) {
		// Decode packet
		nalus, err := h264Decoder.Decode(pkt)
		if err != nil {
			c.logger.Debug("Failed to decode packet", zap.Error(err))
			return
		}

		// Update last frame time
		c.mu.Lock()
		c.lastFrame = time.Now()
		timestamp := c.lastFrame
		c.mu.Unlock()

		// Convert NALUs to frame data
		frameData := c.nalusToFrame(nalus)

		// Call callback if set
		if c.onFrame != nil {
			// Use current time as PTS (in production, extract from RTP packet)
			c.onFrame(frameData, timestamp)
		}

		// Publish frame received event
		if c.eventBus != nil {
			// Extract camera ID from URL or use URL as identifier
			cameraID := c.url // For now, use URL as camera ID (will be improved when camera manager integrates)
			c.eventBus.Publish(evtbusstypes.Event{
				Type:      evtbusstypes.EventType("camera.frame.received"),
				Source:    c.Name(),
				Timestamp: timestamp,
				Data: map[string]interface{}{
					"camera_id":  cameraID,
					"frame_data": frameData,
				},
			})
		}
	})

	// Start playing
	_, err = client.Play(nil)
	if err != nil {
		return fmt.Errorf("failed to play stream: %w", err)
	}

	// Store connection
	c.mu.Lock()
	c.client = client
	c.connected = true
	c.healthStatus = "connected"
	c.mu.Unlock()

	c.logger.Info("RTSP stream connected", zap.String("url", c.url))

	// Publish connection event
	if c.eventBus != nil {
		c.eventBus.Publish(evtbusstypes.Event{
			Type:      evtbusstypes.EventTypeDeviceConnected,
			Source:    c.Name(),
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"url": c.url,
			},
		})
	}

	// Wait for connection to close or error
	go func() {
		err := client.Wait()
		if err != nil {
			c.logger.Error("RTSP stream error", zap.Error(err), zap.String("url", c.url))
		}
		c.mu.Lock()
		c.connected = false
		c.healthStatus = "disconnected"
		c.mu.Unlock()
	}()

	return nil
}

// nalusToFrame converts NALUs to frame data
func (c *RTSPClient) nalusToFrame(nalus [][]byte) []byte {
	// Simple concatenation - in production, you'd want proper H.264 frame assembly
	var frame []byte
	for _, nalu := range nalus {
		frame = append(frame, nalu...)
	}
	return frame
}

// monitorHealth monitors stream health
func (c *RTSPClient) monitorHealth() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.mu.RLock()
			lastFrame := c.lastFrame
			connected := c.connected
			c.mu.RUnlock()

			if !connected {
				return
			}

			// Check if we're receiving frames
			if time.Since(lastFrame) > 30*time.Second {
				c.logger.Info("No frames received for 30 seconds", zap.String("url", c.url))
				c.mu.Lock()
				c.healthStatus = "degraded"
				c.mu.Unlock()

				// Publish health event
				if c.eventBus != nil {
					c.eventBus.Publish(evtbusstypes.Event{
						Type:      evtbusstypes.EventTypeDeviceDisconnected,
						Source:    c.Name(),
						Timestamp: time.Now(),
						Data: map[string]interface{}{
							"url":    c.url,
							"reason": "no_frames",
						},
					})
				}
			} else {
				c.mu.Lock()
				c.healthStatus = "healthy"
				c.mu.Unlock()
			}
		}
	}
}

// IsConnected returns whether the client is connected
func (c *RTSPClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// GetHealthStatus returns the health status
func (c *RTSPClient) GetHealthStatus() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.healthStatus
}

// GetLastFrameTime returns the time of the last received frame
func (c *RTSPClient) GetLastFrameTime() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastFrame
}
