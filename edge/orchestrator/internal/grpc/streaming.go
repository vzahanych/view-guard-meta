package grpc

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"

	edge "github.com/vzahanych/view-guard-meta/proto/go/generated/edge"
)

// StreamingService handles on-demand clip streaming
type StreamingService struct {
	client *Client
	logger *logger.Logger
}

// NewStreamingService creates a new streaming service
func NewStreamingService(client *Client, log *logger.Logger) *StreamingService {
	return &StreamingService{
		client: client,
		logger: log,
	}
}

// StreamClip streams a video clip on-demand to the KVM VM
func (ss *StreamingService) StreamClip(ctx context.Context, eventID string, clipPath string, startOffset int64) error {
	client := ss.client.GetStreamingClient()
	if client == nil {
		return fmt.Errorf("gRPC streaming client not available")
	}

	// Open clip file
	file, err := os.Open(clipPath)
	if err != nil {
		return fmt.Errorf("failed to open clip file: %w", err)
	}
	defer file.Close()

	// Seek to start offset if specified
	if startOffset > 0 {
		if _, err := file.Seek(startOffset, io.SeekStart); err != nil {
			return fmt.Errorf("failed to seek to offset: %w", err)
		}
	}

	// Get file info for size
	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}
	fileSize := fileInfo.Size()

	// Create stream (client-side streaming)
	stream, err := client.StreamClip(ctx)
	if err != nil {
		return fmt.Errorf("failed to create stream: %w", err)
	}

	// Send header as first chunk
	headerChunk := &edge.StreamClipChunk{
		Payload: &edge.StreamClipChunk_Header{
			Header: &edge.StreamClipHeader{
				EventId:     eventID,
				ClipPath:    clipPath,
				StartOffset: startOffset,
				TotalSize:   uint64(fileSize),
			},
		},
		Offset: 0,
		Eof:    false,
	}
	if err := stream.Send(headerChunk); err != nil {
		return fmt.Errorf("failed to send header: %w", err)
	}

	// Stream file in chunks
	chunkSize := 64 * 1024 // 64KB chunks
	buffer := make([]byte, chunkSize)
	offset := startOffset

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Read chunk
		n, err := file.Read(buffer)
		if err == io.EOF {
			// Send final chunk with EOF flag
			if n > 0 {
				dataChunk := &edge.StreamClipChunk{
					Payload: &edge.StreamClipChunk_Data{
						Data: buffer[:n],
					},
					Offset: offset,
					Eof:    true,
				}
				if err := stream.Send(dataChunk); err != nil {
					return fmt.Errorf("failed to send final chunk: %w", err)
				}
			} else {
				// Send EOF chunk
				eofChunk := &edge.StreamClipChunk{
					Payload: &edge.StreamClipChunk_Data{
						Data: nil,
					},
					Offset: offset,
					Eof:    true,
				}
				if err := stream.Send(eofChunk); err != nil {
					return fmt.Errorf("failed to send EOF chunk: %w", err)
				}
			}
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read file: %w", err)
		}

		// Send chunk
		dataChunk := &edge.StreamClipChunk{
			Payload: &edge.StreamClipChunk_Data{
				Data: buffer[:n],
			},
			Offset: offset,
			Eof:    false,
		}
		if err := stream.Send(dataChunk); err != nil {
			return fmt.Errorf("failed to send chunk: %w", err)
		}

		offset += int64(n)

		// Check if we've sent the entire file
		if offset >= fileSize {
			// Send EOF chunk
			eofChunk := &edge.StreamClipChunk{
				Payload: &edge.StreamClipChunk_Data{
					Data: nil,
				},
				Offset: offset,
				Eof:    true,
			}
			if err := stream.Send(eofChunk); err != nil {
				return fmt.Errorf("failed to send EOF chunk: %w", err)
			}
			break
		}
	}

	// Close stream and get response
	resp, err := stream.CloseAndRecv()
	if err != nil && err != io.EOF {
		return fmt.Errorf("failed to close stream: %w", err)
	}

	if resp != nil && !resp.Success {
		return fmt.Errorf("KVM VM rejected stream: %s", resp.ErrorMessage)
	}

	ss.logger.Debug("Clip streamed successfully",
		"event_id", eventID,
		"clip_path", clipPath,
		"size", fileSize,
	)

	return nil
}

// GetClipInfo retrieves information about a clip without streaming
func (ss *StreamingService) GetClipInfo(ctx context.Context, eventID string, clipPath string) (*edge.GetClipInfoResponse, error) {
	client := ss.client.GetStreamingClient()
	
	// If gRPC client is available and connected, try to get info from KVM VM
	if client != nil && ss.client.IsConnected() {
		// Create request
		req := &edge.GetClipInfoRequest{
			EventId:  eventID,
			ClipPath: clipPath,
		}

		// Call gRPC service
		resp, err := client.GetClipInfo(ctx, req)
		if err == nil && resp != nil && resp.Success {
			return resp, nil
		}
		// If gRPC call fails, fall through to local file info
	}

	// Fallback to local file info (always available)
	fileInfo, err := os.Stat(clipPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	// For PoC, we'll return basic info
	// In production, would parse video metadata to get duration and format
	localResp := &edge.GetClipInfoResponse{
		Success:        true,
		SizeBytes:      uint64(fileInfo.Size()),
		DurationSeconds: 0, // Would parse from video metadata
		Format:         "mp4", // Would detect from file extension or metadata
	}

	return localResp, nil
}
