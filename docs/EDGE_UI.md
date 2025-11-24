# Edge Orchestrator Web UI User Guide

This guide provides instructions for using the Edge Orchestrator Web UI to manage and monitor your Edge Appliance.

## Getting Started

### Accessing the Web UI

1. **Find your Edge Appliance IP address**:
   - The IP address is typically displayed during orchestrator startup
   - Or check your router's device list
   - Or run `ip addr show` on the Edge Appliance

2. **Open the web UI**:
   - Open a web browser on any device on your local network
   - Navigate to: `http://<edge-appliance-ip>:8080`
   - Example: `http://192.168.1.100:8080`

3. **Verify connection**:
   - You should see the Dashboard page
   - The system status should show "healthy"

## Dashboard

The Dashboard provides an overview of your Edge Appliance status and metrics.

### System Status

- **Health**: Overall system health status
- **Uptime**: How long the orchestrator has been running
- **Version**: Application version number

### System Metrics

Real-time metrics displayed with progress bars and charts:

- **CPU Usage**: Current CPU utilization percentage
- **Memory Usage**: RAM usage with total and used amounts
- **Disk Usage**: Storage usage with available space

Metrics automatically refresh every 5 seconds. Click the refresh button to manually update.

### Application Metrics

- **Event Queue Length**: Number of events waiting to be processed
- **Active Cameras**: Number of cameras currently streaming
- **Total Cameras**: Total number of registered cameras
- **Enabled Cameras**: Number of cameras that are enabled
- **Online Cameras**: Number of cameras that are currently online

## Camera Viewer

The Camera Viewer allows you to view live video streams from your cameras.

### Viewing a Single Camera

1. Navigate to **Cameras** in the sidebar
2. Select a camera from the dropdown
3. The stream will start automatically
4. Use controls to:
   - **Play/Pause**: Start or stop the stream
   - **Refresh**: Restart the stream
   - **Fullscreen**: View in fullscreen mode

### Viewing Multiple Cameras

1. Click the **Grid View** toggle
2. Select cameras using the checkboxes
3. Selected cameras will appear in a grid layout
4. The grid automatically adjusts based on the number of selected cameras

### Camera Status Indicators

- **Green**: Camera is online and streaming
- **Yellow**: Camera is connecting
- **Red**: Camera is offline or has an error
- **Gray**: Camera is disabled

## Event Timeline

The Event Timeline shows all detected events (motion, objects, etc.) from your cameras.

### Viewing Events

1. Navigate to **Events** in the sidebar
2. Events are displayed in reverse chronological order (newest first)
3. Each event card shows:
   - Camera name
   - Event type
   - Timestamp
   - Confidence score
   - Snapshot thumbnail

### Filtering Events

Use the filter controls to narrow down events:

- **Camera**: Filter by specific camera
- **Event Type**: Filter by event type (motion, person, vehicle, etc.)
- **Date Range**: Filter by date range
- **Confidence**: Filter by minimum confidence score

### Event Details

Click on an event card to view detailed information:

- **Metadata**: Full event metadata including bounding boxes
- **Snapshot**: Full-size snapshot image
- **Video Clip**: Playback of the event video clip (if available)

### Pagination

Events are paginated for performance. Use the pagination controls at the bottom to navigate through pages.

## Configuration

The Configuration page allows you to modify system settings.

### Camera Configuration

- **Recording Enabled**: Enable/disable video recording
- **Motion Detection**: Enable/disable motion detection
- **Quality**: Video quality setting (low, medium, high)
- **Frame Rate**: Frames per second
- **Resolution**: Video resolution

### AI Configuration

- **Service URL**: AI service endpoint
- **Detection Threshold**: Minimum confidence for detections
- **Object Types**: Types of objects to detect
- **Processing Mode**: Real-time or batch processing

### Storage Configuration

- **Clips Directory**: Path for storing video clips
- **Snapshots Directory**: Path for storing snapshots
- **Retention Days**: How long to keep files
- **Max Disk Usage**: Maximum disk usage percentage before cleanup

### WireGuard Configuration

- **Enabled**: Enable/disable WireGuard VPN
- **Config Path**: Path to WireGuard configuration file
- **Interface Name**: Network interface name

### Telemetry Configuration

- **Enabled**: Enable/disable telemetry collection
- **Collection Interval**: How often to collect metrics
- **Send Interval**: How often to send telemetry data

### Encryption Configuration

- **Enabled**: Enable/disable encryption
- **Key Path**: Path to encryption key file
- **Algorithm**: Encryption algorithm to use

### Saving Configuration

1. Make your changes
2. Click **Save** at the bottom of the form
3. A confirmation message will appear
4. Some changes may require a service restart to take effect

## Camera Management

The Camera Management page allows you to add, edit, and remove cameras.

### Viewing Cameras

The camera list shows:
- Camera name and type
- Status (online/offline/enabled/disabled)
- Last seen timestamp
- Action buttons

### Adding a Camera

#### RTSP Camera

1. Click **Add Camera**
2. Select **RTSP** as the camera type
3. Fill in the form:
   - **Name**: Descriptive name for the camera
   - **RTSP URL**: RTSP stream URL (e.g., `rtsp://192.168.1.100/stream`)
   - **Username/Password**: If required
   - **Configuration**: Recording, motion detection, quality settings
4. Click **Save**

#### ONVIF Camera

1. Click **Add Camera**
2. Select **ONVIF** as the camera type
3. Fill in the form:
   - **Name**: Descriptive name
   - **IP Address**: Camera IP address
   - **ONVIF Endpoint**: ONVIF service endpoint (usually auto-detected)
   - **Username/Password**: ONVIF credentials
4. Click **Save**

#### USB Camera

1. Click **Add Camera**
2. Select **USB** as the camera type
3. Fill in the form:
   - **Name**: Descriptive name
   - **Device Path**: USB device path (e.g., `/dev/video0`)
4. Click **Save**

### Editing a Camera

1. Find the camera in the list
2. Click the **Edit** button
3. Modify the settings
4. Click **Save**

### Deleting a Camera

1. Find the camera in the list
2. Click the **Delete** button
3. Confirm the deletion

⚠️ **Warning**: Deleting a camera will remove all associated events and recordings.

### Camera Discovery

The Camera Discovery feature can automatically find cameras on your network.

1. Click **Discover Cameras**
2. Wait for discovery to complete (may take 30-60 seconds)
3. Review discovered cameras
4. Click **Add** next to cameras you want to register

Discovery finds:
- **ONVIF cameras**: Network cameras supporting ONVIF protocol
- **USB cameras**: USB video devices connected to the Edge Appliance

### Testing Camera Connection

1. Find the camera in the list
2. Click the **Test** button
3. Wait for the test to complete
4. Review the test results:
   - **Success**: Camera is reachable and streaming
   - **Failure**: Check camera settings, network connectivity, and credentials

### Enabling/Disabling Cameras

- Click the **Enable/Disable** toggle to start or stop camera streaming
- Disabled cameras won't process video or generate events
- Camera status is preserved when disabled

## Tips and Best Practices

### Performance

- Limit the number of cameras viewed simultaneously in grid view
- Use appropriate video quality settings based on your network bandwidth
- Regularly clean up old events and clips to free up storage

### Network

- Ensure cameras and Edge Appliance are on the same network
- Use wired connections for cameras when possible for better reliability
- Check firewall settings if cameras aren't discoverable

### Storage

- Monitor disk usage on the Dashboard
- Adjust retention days based on available storage
- Set appropriate max disk usage to trigger automatic cleanup

### Troubleshooting

- **Camera not showing**: Check camera status, test connection, verify credentials
- **Streams not loading**: Check FFmpeg installation, camera connectivity, RTSP URL
- **Events not appearing**: Verify motion detection is enabled, check AI service status
- **Configuration not saving**: Check file permissions, verify configuration format

## Keyboard Shortcuts

- **Esc**: Close modals and exit fullscreen
- **Space**: Play/pause video streams (when focused)
- **F11**: Toggle browser fullscreen

## Browser Compatibility

The web UI is tested and works with:
- Chrome/Edge (recommended)
- Firefox
- Safari

For best performance, use a modern browser with JavaScript enabled.

## Getting Help

If you encounter issues:

1. Check the Dashboard for system health status
2. Review camera status in Camera Management
3. Check orchestrator logs for error messages
4. Verify network connectivity and firewall settings
5. Test camera connections individually

For more technical information, see the [API Documentation](../edge/orchestrator/internal/web/README.md).

