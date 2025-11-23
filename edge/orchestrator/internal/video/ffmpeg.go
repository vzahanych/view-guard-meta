package video

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
)

// FFmpegWrapper wraps FFmpeg functionality
type FFmpegWrapper struct {
	logger          *logger.Logger
	ffmpegPath      string
	hardwareAccel   HardwareAcceleration
	availableCodecs map[string]bool
	mu              sync.RWMutex
}

// HardwareAcceleration represents available hardware acceleration
type HardwareAcceleration struct {
	IntelQSV   bool // Intel Quick Sync Video via VAAPI
	NVIDIANVENC bool // NVIDIA NVENC/NVDEC
	Software   bool // Software fallback (always available)
}

// CodecInfo represents codec information
type CodecInfo struct {
	Name        string
	Description string
	Type        string // "video" or "audio"
	Hardware    bool   // Hardware accelerated
}

// NewFFmpegWrapper creates a new FFmpeg wrapper
func NewFFmpegWrapper(log *logger.Logger) (*FFmpegWrapper, error) {
	wrapper := &FFmpegWrapper{
		logger:          log,
		ffmpegPath:      "ffmpeg",
		availableCodecs: make(map[string]bool),
	}

	// Detect FFmpeg installation
	ffmpegPath, err := wrapper.detectFFmpeg()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg not found: %w", err)
	}
	wrapper.ffmpegPath = ffmpegPath

	// Detect hardware acceleration
	hwAccel, err := wrapper.detectHardwareAcceleration()
	if err != nil {
		log.Warn("Failed to detect hardware acceleration, using software fallback", "error", err)
		hwAccel.Software = true
	}
	wrapper.hardwareAccel = hwAccel

	// Detect available codecs
	codecs, err := wrapper.detectCodecs()
	if err != nil {
		log.Warn("Failed to detect codecs", "error", err)
	} else {
		wrapper.availableCodecs = codecs
	}

	log.Info("FFmpeg wrapper initialized",
		"path", wrapper.ffmpegPath,
		"intel_qsv", hwAccel.IntelQSV,
		"nvidia_nvenc", hwAccel.NVIDIANVENC,
		"software", hwAccel.Software,
	)

	return wrapper, nil
}

// detectFFmpeg finds FFmpeg executable
func (f *FFmpegWrapper) detectFFmpeg() (string, error) {
	// Try common paths
	paths := []string{"ffmpeg", "/usr/bin/ffmpeg", "/usr/local/bin/ffmpeg"}

	for _, path := range paths {
		cmd := exec.Command(path, "-version")
		if err := cmd.Run(); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("ffmpeg not found in PATH or common locations")
}

// detectHardwareAcceleration detects available hardware acceleration
func (f *FFmpegWrapper) detectHardwareAcceleration() (HardwareAcceleration, error) {
	accel := HardwareAcceleration{
		Software: true, // Software is always available
	}

	// Check Intel QSV via VAAPI
	if f.checkVAAPI() {
		accel.IntelQSV = true
		f.logger.Info("Intel QSV (VAAPI) hardware acceleration detected")
	}

	// Check NVIDIA NVENC
	if f.checkNVENC() {
		accel.NVIDIANVENC = true
		f.logger.Info("NVIDIA NVENC hardware acceleration detected")
	}

	return accel, nil
}

// checkVAAPI checks if VAAPI (Intel QSV) is available
func (f *FFmpegWrapper) checkVAAPI() bool {
	// Check if VAAPI devices are available
	cmd := exec.Command("vainfo")
	if err := cmd.Run(); err != nil {
		return false
	}

	// Check if FFmpeg supports VAAPI
	cmd = exec.Command(f.ffmpegPath, "-hide_banner", "-encoders")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	outputStr := string(output)
	// Check for VAAPI encoders/decoders
	return strings.Contains(outputStr, "h264_vaapi") || strings.Contains(outputStr, "hevc_vaapi")
}

// checkNVENC checks if NVIDIA NVENC is available
func (f *FFmpegWrapper) checkNVENC() bool {
	// Check if NVIDIA GPU is available
	cmd := exec.Command("nvidia-smi")
	if err := cmd.Run(); err != nil {
		return false
	}

	// Check if FFmpeg supports NVENC
	cmd = exec.Command(f.ffmpegPath, "-hide_banner", "-encoders")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	outputStr := string(output)
	// Check for NVENC encoders
	return strings.Contains(outputStr, "h264_nvenc") || strings.Contains(outputStr, "hevc_nvenc")
}

// detectCodecs detects available codecs
func (f *FFmpegWrapper) detectCodecs() (map[string]bool, error) {
	codecs := make(map[string]bool)

	// Get decoder list
	cmd := exec.Command(f.ffmpegPath, "-hide_banner", "-decoders")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get decoders: %w", err)
	}

	outputStr := string(output)
	lines := strings.Split(outputStr, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, " V") || strings.HasPrefix(line, " A") {
			parts := strings.Fields(line)
			if len(parts) > 1 {
				codecName := parts[1]
				codecs[codecName] = true
			}
		}
	}

	// Get encoder list
	cmd = exec.Command(f.ffmpegPath, "-hide_banner", "-encoders")
	output, err = cmd.Output()
	if err != nil {
		return codecs, nil // Return decoders even if encoders fail
	}

	outputStr = string(output)
	lines = strings.Split(outputStr, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, " V") || strings.HasPrefix(line, " A") {
			parts := strings.Fields(line)
			if len(parts) > 1 {
				codecName := parts[1]
				codecs[codecName] = true
			}
		}
	}

	return codecs, nil
}

// GetHardwareAcceleration returns available hardware acceleration
func (f *FFmpegWrapper) GetHardwareAcceleration() HardwareAcceleration {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.hardwareAccel
}

// IsCodecAvailable checks if a codec is available
func (f *FFmpegWrapper) IsCodecAvailable(codec string) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.availableCodecs[codec]
}

// GetPreferredDecoder returns the preferred decoder for a codec
func (f *FFmpegWrapper) GetPreferredDecoder(codec string) string {
	f.mu.RLock()
	defer f.mu.RUnlock()

	// Prefer hardware acceleration if available
	if codec == "h264" {
		if f.hardwareAccel.IntelQSV {
			return "h264_vaapi"
		}
		if f.hardwareAccel.NVIDIANVENC {
			return "h264_cuvid"
		}
		return "h264"
	}

	if codec == "hevc" || codec == "h265" {
		if f.hardwareAccel.IntelQSV {
			return "hevc_vaapi"
		}
		if f.hardwareAccel.NVIDIANVENC {
			return "hevc_cuvid"
		}
		return "hevc"
	}

	// Default to software decoder
	return codec
}

// GetPreferredEncoder returns the preferred encoder for a codec
func (f *FFmpegWrapper) GetPreferredEncoder(codec string) string {
	f.mu.RLock()
	defer f.mu.RUnlock()

	// Prefer hardware acceleration if available
	if codec == "h264" {
		if f.hardwareAccel.IntelQSV {
			return "h264_vaapi"
		}
		if f.hardwareAccel.NVIDIANVENC {
			return "h264_nvenc"
		}
		return "libx264"
	}

	if codec == "hevc" || codec == "h265" {
		if f.hardwareAccel.IntelQSV {
			return "hevc_vaapi"
		}
		if f.hardwareAccel.NVIDIANVENC {
			return "hevc_nvenc"
		}
		return "libx265"
	}

	// Default to software encoder
	return codec
}

// BuildCommand builds an FFmpeg command with appropriate hardware acceleration
func (f *FFmpegWrapper) BuildCommand(ctx context.Context, args []string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, f.ffmpegPath, args...)
	return cmd
}

// GetVersion returns FFmpeg version
func (f *FFmpegWrapper) GetVersion() (string, error) {
	cmd := exec.Command(f.ffmpegPath, "-version")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get ffmpeg version: %w", err)
	}

	lines := strings.Split(string(output), "\n")
	if len(lines) > 0 {
		return strings.TrimSpace(lines[0]), nil
	}

	return "unknown", nil
}

// ValidateInput validates an input source (RTSP URL or device path)
func (f *FFmpegWrapper) ValidateInput(input string) error {
	// Quick probe to check if input is valid
	args := []string{
		"-hide_banner",
		"-probesize", "32",
		"-analyzeduration", "1000000",
		"-i", input,
		"-f", "null",
		"-",
	}

	cmd := f.BuildCommand(context.Background(), args)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if it's a timeout or actual error
		if strings.Contains(string(output), "Connection refused") ||
			strings.Contains(string(output), "No such file") ||
			strings.Contains(string(output), "Invalid data found") {
			return fmt.Errorf("invalid input: %s: %w", string(output), err)
		}
		// Some warnings are OK, just return the error
		return fmt.Errorf("input validation failed: %w", err)
	}

	return nil
}

