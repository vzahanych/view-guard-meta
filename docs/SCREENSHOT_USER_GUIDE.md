# Screenshot Management User Guide

This guide explains how to use the screenshot management feature in the View Guard Edge Orchestrator for collecting labeled training data.

## Overview

The screenshot management system allows you to:
- Capture screenshots from camera streams
- Label screenshots for training data collection
- Track dataset progress per camera
- Export labeled datasets for model training
- Manage screenshot storage and retention

## Screenshot Capture Workflow

### Step 1: Access the Screenshots Page

1. Open the Edge Web UI in your browser (typically `http://localhost:8080` or your configured host/port)
2. Navigate to the **Screenshots** page from the main menu
3. You will see a list of available cameras and their current dataset status

### Step 2: View Camera Dataset Status

Each camera displays:
- **Labeled Snapshot Count**: Number of labeled screenshots collected
- **Required Snapshot Count**: Minimum number of "normal" labeled snapshots needed (default: 50)
- **Progress Indicator**: Visual progress bar showing collection status
- **Snapshot Required Flag**: Indicates if more screenshots are needed

### Step 3: Capture a Screenshot

1. Select a camera from the list
2. Click the **"Capture"** button next to the camera
3. The system will:
   - Capture a snapshot from the camera's live stream
   - Display the captured image in a preview modal
   - Allow you to label the screenshot

### Step 4: Label the Screenshot

1. In the preview modal, select a label:
   - **Normal**: Typical scene without threats or anomalies
   - **Threat**: Scene containing a detected threat
   - **Abnormal**: Scene with anomalies but not necessarily a threat
   - **Custom**: Custom label (enter a custom label name)

2. Optionally add a description to provide context

3. Optionally specify who created the screenshot (for audit purposes)

4. Click **"Save"** to save the labeled screenshot

### Step 5: Review Dataset Progress

After saving, the dataset status updates automatically:
- The labeled snapshot count increases
- Progress bar updates
- If you've reached the required count (default: 50 normal snapshots), the "Snapshot Required" flag changes to `false`

### Step 6: Manage Screenshots

You can:
- **View**: Click on a screenshot to view full-size image
- **Edit**: Update the label or description of existing screenshots
- **Delete**: Remove screenshots that are no longer needed
- **Filter**: Filter screenshots by camera, label, or date range
- **Export**: Export all labeled screenshots as a dataset for training

## Dataset Progress Calculation

### How Progress is Calculated

The dataset progress is calculated based on:

1. **Label Counts**: Number of screenshots per label type (normal, threat, abnormal, custom)
2. **Required Count**: Minimum number of "normal" labeled snapshots (configurable, default: 50)
3. **Snapshot Required Flag**: 
   - `true` if `labeled_snapshot_count < required_snapshot_count`
   - `false` if `labeled_snapshot_count >= required_snapshot_count`

### Progress Formula

```
Progress = (labeled_snapshot_count / required_snapshot_count) * 100%
```

Example:
- If you have 25 normal labeled snapshots and the required count is 50:
  - Progress = (25 / 50) * 100% = 50%
  - `snapshot_required` = `true`

- If you have 50 or more normal labeled snapshots:
  - Progress = 100% (capped)
  - `snapshot_required` = `false`

### Requirements

**Minimum Requirements**:
- At least 50 "normal" labeled snapshots per camera (configurable via `min_normal_snapshots`)
- Screenshots must be properly labeled to be counted

**Best Practices**:
- Collect a diverse set of "normal" scenes (different times of day, weather conditions, etc.)
- Include examples of threats and anomalies for better model training
- Regularly review and update labels as needed
- Export datasets periodically for backup and training

## Label Types

### Normal
- **Purpose**: Baseline scenes showing typical activity
- **Use Case**: Training the model to recognize normal patterns
- **Minimum Required**: 50 (configurable)
- **Examples**: Empty parking lot, normal hallway activity, typical office scene

### Threat
- **Purpose**: Scenes containing detected threats
- **Use Case**: Training the model to identify threats
- **Minimum Required**: None (but recommended for balanced training)
- **Examples**: Unauthorized person, suspicious activity, security breach

### Abnormal
- **Purpose**: Scenes with anomalies but not necessarily threats
- **Use Case**: Training the model to detect unusual patterns
- **Minimum Required**: None (but recommended for balanced training)
- **Examples**: Unusual lighting, unexpected objects, environmental changes

### Custom
- **Purpose**: User-defined categories
- **Use Case**: Specific scenarios unique to your environment
- **Minimum Required**: None
- **Examples**: "Delivery", "Maintenance", "Special Event"

## Dataset Export

### Exporting Screenshots

1. Navigate to the Screenshots page
2. Apply filters if needed (camera, label, date range)
3. Click **"Export Dataset"**
4. The system will:
   - Create a ZIP archive containing all filtered screenshots
   - Include metadata (labels, descriptions, timestamps)
   - Generate a manifest file with dataset information
   - Download the archive to your computer

### Export Format

The exported dataset includes:
- Screenshot images (JPEG format, optimized)
- `manifest.json`: Dataset metadata and structure
- `labels.csv`: Label mapping and counts
- `README.txt`: Export information and instructions

### Using Exported Datasets

Exported datasets can be used for:
- Model training and retraining
- Dataset backup and archival
- Sharing with other systems
- Compliance and audit purposes

## Storage Management

### Storage Limits

You can configure:
- **Per-screenshot size limit**: Maximum size per screenshot (default: no limit)
- **Total storage limit**: Maximum total size for all screenshots (default: no limit)
- **Retention policy**: Automatic deletion after specified days (default: no automatic deletion)

See [Screenshot Configuration Guide](./SCREENSHOT_CONFIGURATION.md) for details.

### Disk Usage Monitoring

The system monitors disk usage and:
- Warns when approaching configured thresholds
- Prevents saving new screenshots if disk is full
- Provides storage statistics per camera

### Cleanup

Automatic cleanup (if enabled):
- Deletes screenshots older than the retention period
- Removes orphaned files (files without database records)
- Maintains storage within configured limits

## Troubleshooting

### Screenshot Capture Fails

**Problem**: Cannot capture screenshot from camera

**Solutions**:
1. Verify camera is enabled and streaming
2. Check camera connection status
3. Ensure camera has video streams available
4. Check system logs for errors

### Dataset Progress Not Updating

**Problem**: Progress bar or counts not updating after saving screenshot

**Solutions**:
1. Refresh the page
2. Check that the screenshot was saved successfully
3. Verify the label is "normal" (only normal labels count toward required count)
4. Check system logs for errors

### Storage Issues

**Problem**: Cannot save screenshots due to storage errors

**Solutions**:
1. Check available disk space
2. Review storage configuration limits
3. Delete old or unnecessary screenshots
4. Adjust retention policy if needed
5. Check disk permissions

### Export Fails

**Problem**: Dataset export fails or is incomplete

**Solutions**:
1. Check available disk space for export directory
2. Verify export directory permissions
3. Try exporting a smaller subset (use filters)
4. Check system logs for specific errors

## Best Practices

1. **Regular Collection**: Collect screenshots regularly to build a comprehensive dataset
2. **Diverse Scenarios**: Include screenshots from different times, conditions, and scenarios
3. **Accurate Labeling**: Ensure labels accurately reflect the scene content
4. **Review and Update**: Periodically review and update labels as needed
5. **Backup Exports**: Export datasets regularly for backup and training
6. **Monitor Storage**: Keep an eye on storage usage and adjust limits as needed
7. **Document Context**: Use descriptions to provide context for unusual screenshots

## Related Documentation

- [Screenshot Configuration Guide](./SCREENSHOT_CONFIGURATION.md)
- [Screenshot Testing Guide](./TESTING_SCREENSHOTS.md)
- [API Documentation](./SCREENSHOT_API.md)
- [Implementation Plan Phase 2](./IMPLEMENTATION_PLAN_PHASE2.md)
