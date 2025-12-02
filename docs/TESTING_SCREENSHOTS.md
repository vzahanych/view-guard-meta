# Screenshot Management Testing Guide

This guide provides comprehensive testing instructions for the screenshot management functionality (Step 2.2.2.6).

## Table of Contents

1. [Manual Testing](#manual-testing)
2. [Automated Testing](#automated-testing)
3. [Test Scenarios](#test-scenarios)

## Manual Testing

### Prerequisites

1. Start the edge orchestrator service:
   ```bash
   cd edge/orchestrator
   go run cmd/orchestrator/main.go
   ```

2. Ensure at least one camera is configured and enabled

3. Access the web UI at `http://localhost:8080` (or configured port)

### Substep 2.2.2.6.1: Test Snapshot Capture Workflow

#### Test 1: Capture Multiple Snapshots in Sequence

1. Navigate to the Screenshots page
2. Select a camera from the dropdown
3. Click "Capture Screenshot" multiple times (at least 5 times)
4. For each capture:
   - Select a label (normal, threat, abnormal, or custom)
   - If custom, enter a custom label
   - Optionally add a description
   - Click "Save Screenshot"
5. **Expected Results:**
   - Each screenshot is saved successfully
   - Success toast notification appears
   - Screenshot appears in the list immediately
   - Dataset progress updates after each save

#### Test 2: Save Snapshots with Different Labels

1. Capture 4 screenshots with different labels:
   - One with "normal" label
   - One with "threat" label
   - One with "abnormal" label
   - One with "custom" label (provide custom label text)
2. **Expected Results:**
   - All screenshots are saved successfully
   - Each screenshot shows the correct label badge
   - Custom label screenshot shows the custom label text
   - Dataset progress reflects only "normal" labeled snapshots

#### Test 3: Verify Dataset Progress Updates Immediately After Save

1. Note the current dataset progress (e.g., "5/50 normal snapshots")
2. Capture and save a new screenshot with "normal" label
3. **Expected Results:**
   - Progress immediately updates to "6/50 normal snapshots"
   - Progress bar updates visually
   - No page refresh required

#### Test 4: Verify Progress Bars and Counts are Accurate

1. Capture multiple screenshots with "normal" label
2. Check the progress bar and count display
3. **Expected Results:**
   - Progress bar shows correct percentage (e.g., 10/50 = 20%)
   - Count display shows "10 / 50"
   - Progress bar color changes when reaching 100%
   - "Ready for training" message appears when count reaches required amount

### Substep 2.2.2.6.2: Test Dataset Status Sync

#### Test 1: Verify Dataset Status is Synced to VM After Saving

**Note:** This requires a running User VM or mock VM endpoint.

1. Capture and save a screenshot with "normal" label
2. Check the dataset status sync mechanism
3. **Expected Results:**
   - Dataset status is sent to VM (check VM logs or API)
   - Status includes updated snapshot count
   - Sync happens immediately after save

#### Test 2: Verify Periodic Sync Still Works Correctly

1. Wait for the periodic sync interval (check configuration)
2. Make changes to screenshots (add/delete)
3. **Expected Results:**
   - Periodic sync still occurs at configured interval
   - Status is synced even without immediate trigger
   - No conflicts between immediate and periodic sync

#### Test 3: Verify Immediate Sync Trigger Works

1. Capture and save a screenshot
2. Check sync logs immediately
3. **Expected Results:**
   - Immediate sync is triggered
   - Sync completes before next periodic sync
   - Status is accurate in VM

### Substep 2.2.2.6.3: Test Screenshot Management Features

#### Test 1: View Screenshot Details

1. Click on a screenshot thumbnail or "View Details" button
2. **Expected Results:**
   - Detail modal opens
   - Full-size image is displayed
   - All metadata is shown (camera, label, description, dates, metadata JSON)
   - Metadata section is collapsible
   - Quick info badges are displayed

#### Test 2: Edit Screenshot Labels and Metadata

1. Click "Edit" on a screenshot
2. Change the label to a different value
3. Update the description
4. Modify metadata JSON
5. Click "Save Changes"
6. **Expected Results:**
   - Changes are saved successfully
   - Success toast appears
   - Screenshot list updates
   - Dataset progress updates if label changed to/from "normal"

#### Test 3: Delete Screenshots

1. Click "Delete" on a screenshot
2. Confirm deletion in the confirmation dialog
3. **Expected Results:**
   - Confirmation dialog shows screenshot preview
   - Deletion succeeds
   - Success toast appears
   - Screenshot is removed from list
   - Dataset progress updates
   - Undo notification appears (if implemented)

#### Test 4: Filter and Sort Screenshots

1. Use filter dropdowns:
   - Filter by label (normal, threat, abnormal, custom)
   - Filter by camera
   - Search by description
2. Use sort options:
   - Sort by date created/updated
   - Sort by camera
   - Sort by label
   - Change sort order (ascending/descending)
3. **Expected Results:**
   - Filters work correctly
   - Search is debounced (waits 300ms before searching)
   - Sort order is applied correctly
   - Results update immediately

#### Test 5: Test Pagination with Large Datasets

1. Create or ensure you have more than 12 screenshots
2. Navigate through pages
3. Change page size (6, 12, 24, 48)
4. **Expected Results:**
   - Pagination controls work correctly
   - Page numbers are accurate
   - Page size changes apply immediately
   - Previous/Next buttons are disabled appropriately

#### Test 6: Verify Dataset Progress Updates After Edits/Deletions

1. Note current progress (e.g., "10/50 normal snapshots")
2. Edit a screenshot: change label from "normal" to "threat"
3. **Expected Results:**
   - Progress updates to "9/50 normal snapshots"
4. Delete a "normal" labeled screenshot
5. **Expected Results:**
   - Progress updates to "8/50 normal snapshots"
6. Edit a "threat" screenshot: change label to "normal"
7. **Expected Results:**
   - Progress updates to "9/50 normal snapshots"

### Substep 2.2.2.8.4: Test Dataset Status APIs and Manual Sync

#### Test 1: Edge dataset status endpoints

1. Capture several `"normal"` screenshots for a specific camera (e.g., 5–10).
2. Call the Edge dataset status endpoint:
   ```bash
   curl http://localhost:8080/api/cameras/{camera_id}/dataset | jq .
   ```
3. **Expected Results:**
   - `dataset_status.label_counts.normal` reflects the number of `"normal"` screenshots.
   - `required_snapshot_count` equals the configured `min_normal_snapshots` (default `50`).
   - `snapshot_required` is `true` until `labeled_snapshot_count >= required_snapshot_count`.

#### Test 2: Manual Edge → VM dataset sync endpoint (ready/not ready)

1. Ensure the selected camera has **fewer** than the required number of `"normal"` screenshots (e.g., 10 when required is 50).
2. Call:
   ```bash
   curl -X POST http://localhost:8080/api/cameras/{camera_id}/dataset/sync | jq .
   ```
3. **Expected Results (Not Ready):**
   - HTTP status is `409 Conflict`.
   - Response includes:
     - `"error": "Dataset not ready for training (snapshot_required=true)"`
     - `"dataset_status.snapshot_required": true`
     - `"required_snapshots"` equal to `required_snapshot_count`.
4. Capture additional `"normal"` screenshots until `labeled_snapshot_count >= required_snapshot_count` (e.g., reach 50).
5. Call the same sync endpoint again.
6. **Expected Results (Ready):**
   - HTTP status is `200 OK`.
   - Response includes:
     - `"dataset_synced": false` (VM push deferred to a later phase).
     - `"dataset_status.snapshot_required": false`.
     - Up-to-date `label_counts.normal` and `labeled_snapshot_count`.

### Substep 2.2.2.6.4: Test Edge Cases

#### Test 1: No Existing Snapshots

1. Start with a fresh camera (no screenshots)
2. Check dataset progress display
3. **Expected Results:**
   - Progress shows "0 / 50 normal snapshots"
   - Progress bar shows 0%
   - "In Progress" status is shown

#### Test 2: Exactly Required Count

1. Capture exactly the required number of "normal" snapshots (e.g., 50)
2. Check dataset status
3. **Expected Results:**
   - Progress shows "50 / 50 normal snapshots"
   - Progress bar shows 100%
   - "Ready for training" status is shown
   - `snapshot_required` flag is false

#### Test 3: More Than Required Count

1. Capture more than required (e.g., 55 normal snapshots when required is 50)
2. Check dataset status
3. **Expected Results:**
   - Progress shows "55 / 50 normal snapshots"
   - Progress bar shows 100% (capped)
   - "Ready for training" status is shown
   - `snapshot_required` flag is false

#### Test 4: Multiple Cameras

1. Create screenshots for multiple cameras
2. Switch between cameras in the filter
3. **Expected Results:**
   - Each camera shows its own dataset progress
   - Progress is calculated per camera
   - Filtering by camera works correctly

#### Test 5: Different Label Types

1. Create screenshots with all label types:
   - Normal
   - Threat
   - Abnormal
   - Custom (multiple different custom labels)
2. **Expected Results:**
   - All labels are saved correctly
   - Label counts are accurate
   - Filtering by each label works
   - Custom labels are displayed correctly

#### Test 6: Edit Screenshot with Missing Metadata

1. Find or create a screenshot without metadata
2. Edit it and add metadata
3. **Expected Results:**
   - Edit succeeds
   - Metadata is saved correctly
   - Metadata displays in detail view

#### Test 7: Delete Screenshot That No Longer Exists

1. Note a screenshot ID
2. Manually delete the file from disk (or use another method)
3. Try to delete via UI
4. **Expected Results:**
   - Error message is shown
   - Error toast notification appears
   - Operation can be retried

## Automated Testing

### Running Backend Unit Tests

#### Screenshot Service Tests

```bash
cd edge/orchestrator
go test -v ./internal/web/screenshots/... -run TestSaveScreenshot
go test -v ./internal/web/screenshots/... -run TestGetScreenshot
go test -v ./internal/web/screenshots/... -run TestListScreenshots
go test -v ./internal/web/screenshots/... -run TestUpdateScreenshot
go test -v ./internal/web/screenshots/... -run TestDeleteScreenshot
go test -v ./internal/web/screenshots/... -run TestGetLabelCounts
go test -v ./internal/web/screenshots/... -run TestExportDataset
```

#### HTTP Handler Tests

```bash
cd edge/orchestrator
go test -v ./internal/web/... -run TestHandleSaveScreenshot
go test -v ./internal/web/... -run TestHandleListScreenshots
go test -v ./internal/web/... -run TestHandleGetScreenshot
go test -v ./internal/web/... -run TestHandleUpdateScreenshot
go test -v ./internal/web/... -run TestHandleDeleteScreenshot
go test -v ./internal/web/... -run TestHandleExportScreenshots
```

#### Run All Screenshot Tests

```bash
cd edge/orchestrator
go test -v ./internal/web/screenshots/...
go test -v ./internal/web/... -run Screenshot
```

### Test Coverage

Generate test coverage report:

```bash
cd edge/orchestrator
go test -v -coverprofile=coverage.out ./internal/web/screenshots/...
go tool cover -html=coverage.out -o coverage.html
```

## Test Scenarios

### Complete Workflow Test

1. **Setup:**
   - Start with empty database
   - Configure one camera

2. **Capture Phase:**
   - Capture 10 screenshots with "normal" label
   - Capture 5 screenshots with "threat" label
   - Capture 3 screenshots with "abnormal" label
   - Capture 2 screenshots with custom labels

3. **Management Phase:**
   - View details of each screenshot type
   - Edit labels on some screenshots
   - Delete a few screenshots
   - Filter and sort the list

4. **Export Phase:**
   - Export all screenshots
   - Export only "normal" labeled screenshots
   - Export with metadata

5. **Verification:**
   - Check dataset progress is accurate
   - Verify all files exist on disk
   - Verify database records are correct
   - Check export ZIP files contain correct data

### Performance Test

1. **Large Dataset:**
   - Create 100+ screenshots
   - Test pagination performance
   - Test filtering performance
   - Test lazy loading of thumbnails

2. **Concurrent Operations:**
   - Capture multiple screenshots simultaneously
   - Edit multiple screenshots in parallel
   - Test bulk operations

### Error Handling Test

1. **Network Errors:**
   - Simulate network failures
   - Verify retry mechanisms work
   - Check error messages are user-friendly

2. **File System Errors:**
   - Test with full disk
   - Test with permission errors
   - Test with corrupted files

3. **Database Errors:**
   - Test with locked database
   - Test with invalid data

## Troubleshooting

### Common Issues

1. **Screenshots not appearing:**
   - Check browser console for errors
   - Verify API endpoints are accessible
   - Check database for records

2. **Progress not updating:**
   - Check dataset status calculation
   - Verify camera ID matches
   - Check sync service logs

3. **Images not loading:**
   - Verify file paths are correct
   - Check file permissions
   - Verify thumbnail generation

### Debug Mode

Enable debug logging:

```bash
# Set log level to debug in config
# Or set environment variable
LOG_LEVEL=debug go run cmd/orchestrator/main.go
```

## Test Checklist

- [ ] Capture workflow works for all label types
- [ ] Dataset progress updates correctly
- [ ] Screenshot details display correctly
- [ ] Edit functionality works
- [ ] Delete functionality works with confirmation
- [ ] Filtering works for all filters
- [ ] Sorting works for all sort options
- [ ] Pagination works correctly
- [ ] Edge cases are handled gracefully
- [ ] Error messages are clear
- [ ] Loading states are shown
- [ ] Success notifications appear
- [ ] Bulk operations work
- [ ] Export functionality works
- [ ] Keyboard shortcuts work
- [ ] Accessibility features work (ARIA labels)
- [ ] Performance is acceptable with large datasets
