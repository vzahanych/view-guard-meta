# Epic 2.4 Flow Analysis: Current vs. Desired Implementation

## User's Desired Flow

1. **After cameras sync (Epic 2.3)**:
   - Central VM should **ask Edge to take labeled screenshots**
   - User sees that request on Edge UI
   - User takes and labels screenshots manually
   - Integration test should do this automatically

2. **When labeled dataset is ready**:
   - User presses button on Edge UI to **train model**
   - Edge syncs that dataset with VM

## Current Implementation Analysis

### ✅ What's Implemented

#### Epic 2.3: Camera Discovery & Capability Sync
- ✅ Edge discovers cameras independently (USB/ONVIF)
- ✅ Edge syncs camera inventory to VM after authentication
- ✅ VM tracks camera dataset readiness status (`needs_snapshots`, `ready_for_training`)

#### Epic 2.4: Snapshot Capture & Dataset Progress
- ✅ Edge can capture snapshots from cameras
- ✅ Edge can save labeled screenshots (normal, threat, abnormal, custom)
- ✅ Edge tracks dataset status locally (labeled count, required count, training eligibility)
- ✅ Edge UI shows dataset progress and notifications for cameras needing snapshots
- ✅ Edge UI has "Capture Now" button for cameras needing snapshots
- ✅ Edge UI shows progress bars and badges

#### Epic 2.5: Dataset Sync & Upload
- ✅ Edge has `POST /api/cameras/{id}/dataset/sync` endpoint
- ✅ When user presses "Sync Dataset Status" button:
  - Validates dataset readiness (≥50 normal snapshots)
  - Packages all labeled screenshots into tar.gz archive
  - Uploads dataset to VM via HTTP multipart upload
  - Syncs capabilities to VM (updates training eligibility status)
- ✅ VM receives and stores datasets
- ✅ VM updates training eligibility status after dataset upload

### ❌ What's Missing

#### 1. VM → Edge Request for Labeled Screenshots

**Current State:**
- Edge discovers cameras independently
- Edge UI shows notifications/badges for cameras needing snapshots
- User manually takes screenshots via Edge UI
- **No mechanism for VM to actively request Edge to take labeled screenshots**

**Desired State:**
- After cameras sync, VM should send a command/request to Edge asking it to take labeled screenshots
- Edge UI should display this request prominently
- Integration test should automatically trigger snapshot capture when VM requests it

**Missing Components:**
- ❌ VM-side gRPC method to request snapshot capture from Edge
- ❌ Edge-side gRPC handler to receive snapshot capture requests from VM
- ❌ Edge UI component to display VM requests for snapshots
- ❌ Integration test automation for VM → Edge snapshot requests

**Potential Implementation:**
- Add `RequestSnapshotCapture` RPC to `ControlService` proto
- VM calls this RPC when camera needs snapshots (`training_eligibility_status == "needs_snapshots"`)
- Edge receives request and displays notification in UI
- Edge can optionally auto-capture snapshots (for integration tests) or prompt user

#### 2. Training Button Flow Clarity

**Current State:**
- Edge UI has "Sync Dataset Status" button (`POST /api/cameras/{id}/dataset/sync`)
- This button:
  - Validates dataset readiness
  - Uploads dataset to VM
  - Syncs capabilities to VM
- **Button label may not clearly indicate it's for training**

**Desired State:**
- User presses button on Edge UI to **train model**
- Edge syncs dataset with VM
- VM receives dataset and prepares it for training

**Gap:**
- The "Sync Dataset Status" button functionally does what's needed (uploads dataset for training)
- However, the button name and flow may not clearly communicate "this triggers training preparation"
- May need to rename button to "Train Model" or "Prepare for Training" for clarity

**Recommendation:**
- Rename "Sync Dataset Status" button to "Train Model" or "Prepare for Training"
- Update button tooltip/help text to clarify it uploads dataset to VM for training
- Ensure VM-side training pipeline is triggered after dataset upload (Epic 2.7)

## Implementation Plan Recommendations

### Priority 1: VM → Edge Snapshot Request Flow

**Step 1: Add gRPC Proto Definition**
```protobuf
// In proto/proto/edge/control.proto
service ControlService {
  // ... existing methods ...
  
  // Request snapshot capture from Edge
  rpc RequestSnapshotCapture(RequestSnapshotCaptureRequest) returns (RequestSnapshotCaptureResponse);
}

message RequestSnapshotCaptureRequest {
  string camera_id = 1;
  string label = 2;  // Optional: "normal", "threat", "abnormal", "custom"
  string custom_label = 3;  // Required if label == "custom"
  int32 count = 4;  // Number of snapshots to capture (default: 1)
  bool auto_capture = 5;  // If true, Edge auto-captures without user interaction (for tests)
}

message RequestSnapshotCaptureResponse {
  bool accepted = 1;
  string message = 2;
  repeated string snapshot_ids = 3;  // If auto_capture=true, returns captured snapshot IDs
}
```

**Step 2: Implement VM-side Request Handler**
- Location: `user-vm-api/internal/tunnel-gateway/edge_api.go`
- When camera `training_eligibility_status == "needs_snapshots"`, VM calls `RequestSnapshotCapture` RPC
- Can be triggered:
  - Automatically after capability sync detects camera needs snapshots
  - Manually via VM API endpoint (for testing)

**Step 3: Implement Edge-side Request Handler**
- Location: `edge/orchestrator/internal/capabilities/sync_service.go` or new service
- Receives `RequestSnapshotCapture` RPC call from VM
- If `auto_capture=true`: Automatically captures snapshots (for integration tests)
- If `auto_capture=false`: Displays notification in Edge UI prompting user to capture

**Step 4: Edge UI Notification Component**
- Location: `edge/orchestrator/internal/web/frontend/src/pages/Screenshots.tsx`
- Display prominent notification when VM requests snapshots
- Show: Camera name, requested label, count needed
- "Capture Now" button that opens capture modal with pre-filled label

**Step 5: Integration Test Automation**
- Location: `infra/local/scripts/epic_2_4_tests.sh`
- After Epic 2.3 (camera sync), test should:
  - VM calls `RequestSnapshotCapture` with `auto_capture=true`
  - Edge automatically captures and labels snapshots
  - Verify snapshots are saved and dataset status updates

### Priority 2: Training Button Clarity

**Step 1: Rename Button**
- Change "Sync Dataset Status" to "Train Model" or "Prepare for Training"
- Update button tooltip: "Upload dataset to VM and prepare for model training"

**Step 2: Update Documentation**
- Clarify in Phase 2 plan that this button triggers training preparation
- Document the full flow: Capture → Label → Train Button → Upload → Training

## Files to Review/Update

### Phase 2 Implementation Plan
- `/docs/IMPLEMENTATION_PLAN_PHASE2.md`
  - Epic 2.4: Add section on VM → Edge snapshot request flow
  - Epic 2.4: Clarify training button flow and naming

### Proto Definitions
- `/proto/proto/edge/control.proto`
  - Add `RequestSnapshotCapture` RPC method

### VM-side Implementation
- `/user-vm-api/internal/tunnel-gateway/edge_api.go`
  - Add `RequestSnapshotCapture` handler
  - Add automatic request logic when camera needs snapshots

### Edge-side Implementation
- `/edge/orchestrator/internal/capabilities/sync_service.go` or new service
  - Add `RequestSnapshotCapture` RPC handler
  - Implement auto-capture logic for integration tests

### Edge UI
- `/edge/orchestrator/internal/web/frontend/src/pages/Screenshots.tsx`
  - Add notification component for VM snapshot requests
  - Update button label from "Sync Dataset Status" to "Train Model"

### Integration Tests
- `/infra/local/scripts/epic_2_4_tests.sh`
  - Add test for VM → Edge snapshot request flow
  - Add test for automatic snapshot capture (integration test mode)

## Summary

**Current Implementation:**
- ✅ Edge can capture and save labeled screenshots
- ✅ Edge can upload dataset to VM when user presses "Sync Dataset Status" button
- ✅ VM receives and stores datasets
- ❌ **Missing: VM → Edge request mechanism for labeled screenshots**
- ⚠️ **Unclear: Training button naming and flow clarity**

**Recommended Next Steps:**
1. Implement VM → Edge snapshot request flow (gRPC RPC + UI notification)
2. Rename "Sync Dataset Status" button to "Train Model" for clarity
3. Update Phase 2 implementation plan to document the complete flow
4. Add integration test automation for VM → Edge snapshot requests
