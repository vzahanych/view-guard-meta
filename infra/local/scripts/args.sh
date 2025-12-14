#!/bin/bash
# Command-line argument parsing functions

# Source dependencies (SCRIPTS_DIR should be set by calling script)
SCRIPTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPTS_DIR}/logging.sh"

# Test execution flags must be defined by the calling script:
# RUN_EPIC_2_0, RUN_EPIC_2_1, RUN_EPIC_2_2, RUN_EPIC_2_3, RUN_EPIC_2_4, RUN_EPIC_2_5, RUN_EPIC_2_6, RUN_EPIC_2_7, RUN_EPIC_2_8, RUN_EPIC_2_9
# SKIP_CLEANUP, REBUILD, SHOW_HELP

parse_args() {
    local run_all=false
    
    while [[ $# -gt 0 ]]; do
        case $1 in
            --all)
                run_all=true
                shift
                ;;
            --epic)
                if [[ -z "${2:-}" ]]; then
                    log_error " --epic requires an epic number (2.0, 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 2.7, 2.8, or 2.9)"
                    exit 1
                fi
                case "$2" in
                    2.0)
                        RUN_EPIC_2_0=true
                        ;;
                    2.1)
                        RUN_EPIC_2_1=true
                        ;;
                    2.2)
                        RUN_EPIC_2_2=true
                        ;;
                    2.3)
                        RUN_EPIC_2_3=true
                        ;;
                    2.4)
                        RUN_EPIC_2_4=true
                        ;;
                    2.5)
                        RUN_EPIC_2_5=true
                        ;;
                    2.6)
                        RUN_EPIC_2_6=true
                        ;;
                    2.7)
                        RUN_EPIC_2_7=true
                        ;;
                    2.8)
                        RUN_EPIC_2_8=true
                        ;;
                    2.9)
                        RUN_EPIC_2_9=true
                        ;;
                    *)
                        log_error "Invalid epic number: $2 (must be 2.0, 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 2.7, 2.8, or 2.9)"
                        exit 1
                        ;;
                esac
                shift 2
                ;;
            --skip-cleanup)
                SKIP_CLEANUP=true
                CLEANUP_ON_EXIT=false
                shift
                ;;
            --rebuild)
                REBUILD=true
                shift
                ;;
            --help|-h)
                SHOW_HELP=true
                shift
                ;;
            *)
                log_error "Unknown option: $1"
                echo ""
                show_help
                exit 1
                ;;
        esac
    done
    
    # If no specific epics selected, run all
    if [ "$run_all" = "true" ] || ([ "$RUN_EPIC_2_0" = "false" ] && [ "$RUN_EPIC_2_1" = "false" ] && [ "$RUN_EPIC_2_2" = "false" ] && [ "$RUN_EPIC_2_3" = "false" ] && [ "$RUN_EPIC_2_4" = "false" ] && [ "$RUN_EPIC_2_5" = "false" ] && [ "$RUN_EPIC_2_6" = "false" ] && [ "$RUN_EPIC_2_7" = "false" ] && [ "$RUN_EPIC_2_8" = "false" ] && [ "$RUN_EPIC_2_9" = "false" ]); then
        RUN_EPIC_2_0=true
        RUN_EPIC_2_1=true
        RUN_EPIC_2_2=true
        RUN_EPIC_2_3=true
        RUN_EPIC_2_4=true
        RUN_EPIC_2_5=true
        RUN_EPIC_2_6=true
        RUN_EPIC_2_7=true
        RUN_EPIC_2_8=true
        RUN_EPIC_2_9=true
    else
        # Enable all prerequisite epics based on which epics are requested
        # Epic dependencies: 2.0 → 2.1 → 2.2 → 2.3 → 2.4 → 2.5 → 2.6 → 2.7 → 2.8 → 2.9
        # Epics 2.2+ require 2.0 and 2.1 (environment setup)
        if [ "$RUN_EPIC_2_2" = "true" ] || [ "$RUN_EPIC_2_3" = "true" ] || [ "$RUN_EPIC_2_4" = "true" ] || [ "$RUN_EPIC_2_5" = "true" ] || [ "$RUN_EPIC_2_6" = "true" ] || [ "$RUN_EPIC_2_7" = "true" ] || [ "$RUN_EPIC_2_8" = "true" ] || [ "$RUN_EPIC_2_9" = "true" ]; then
            RUN_EPIC_2_0=true
            RUN_EPIC_2_1=true
        fi
        # Epic 2.1 requires 2.0
        if [ "$RUN_EPIC_2_1" = "true" ]; then
            RUN_EPIC_2_0=true
        fi
        # If 2.9 is selected, run all prerequisites
        if [ "$RUN_EPIC_2_9" = "true" ]; then
            RUN_EPIC_2_2=true
            RUN_EPIC_2_3=true
            RUN_EPIC_2_4=true
            RUN_EPIC_2_5=true
            RUN_EPIC_2_6=true
            RUN_EPIC_2_7=true
            RUN_EPIC_2_8=true
        fi
        # If 2.8 is selected, run 2.2, 2.3, 2.4, 2.5, 2.6, and 2.7
        if [ "$RUN_EPIC_2_8" = "true" ]; then
            RUN_EPIC_2_2=true
            RUN_EPIC_2_3=true
            RUN_EPIC_2_4=true
            RUN_EPIC_2_5=true
            RUN_EPIC_2_6=true
            RUN_EPIC_2_7=true
        fi
        # If 2.7 is selected, run 2.2, 2.3, 2.4, 2.5, and 2.6
        if [ "$RUN_EPIC_2_7" = "true" ]; then
            RUN_EPIC_2_2=true
            RUN_EPIC_2_3=true
            RUN_EPIC_2_4=true
            RUN_EPIC_2_5=true
            RUN_EPIC_2_6=true
        fi
        # If 2.6 is selected, run 2.2, 2.3, 2.4, and 2.5
        if [ "$RUN_EPIC_2_6" = "true" ]; then
            RUN_EPIC_2_2=true
            RUN_EPIC_2_3=true
            RUN_EPIC_2_4=true
            RUN_EPIC_2_5=true
        fi
        # If 2.5 is selected, run 2.2, 2.3, and 2.4
        if [ "$RUN_EPIC_2_5" = "true" ]; then
            RUN_EPIC_2_2=true
            RUN_EPIC_2_3=true
            RUN_EPIC_2_4=true
        fi
        # If 2.4 is selected, run 2.2 and 2.3
        if [ "$RUN_EPIC_2_4" = "true" ]; then
            RUN_EPIC_2_2=true
            RUN_EPIC_2_3=true
        fi
        # If 2.3 is selected, run 2.2
        if [ "$RUN_EPIC_2_3" = "true" ]; then
            RUN_EPIC_2_2=true
        fi
        # Epic 2.2 has no prerequisites beyond 2.0 and 2.1 (already handled above)
    fi
}

show_help() {
    cat << EOF
Phase 2 Integration Test Suite

Usage:
  $0 [OPTIONS]

Options:
  --all                    Run all epics (default if no --epic specified)
  --epic <number>          Run specific epic and all prerequisites
                          Epic dependencies: 2.0 → 2.1 → 2.2 → 2.3 → 2.4 → 2.5 → 2.6 → 2.7 → 2.8 → 2.9
                          Examples:
                            --epic 2.0  # Runs only Epic 2.0 (Configuration Generation)
                            --epic 2.1  # Runs Epic 2.0 + 2.1 (Configuration + VM Services)
                            --epic 2.2  # Runs Epic 2.0 + 2.1 + 2.2 (Configuration + VM Services + Connection)
                            --epic 2.3  # Runs Epic 2.0 + 2.1 + 2.2 + 2.3
                            --epic 2.4  # Runs Epic 2.0 + 2.1 + 2.2 + 2.3 + 2.4
                            --epic 2.5  # Runs Epic 2.0 + 2.1 + 2.2 + 2.3 + 2.4 + 2.5
                            --epic 2.6  # Runs Epic 2.0 + 2.1 + 2.2 + 2.3 + 2.4 + 2.5 + 2.6
                            --epic 2.7  # Runs Epic 2.0 + 2.1 + 2.2 + 2.3 + 2.4 + 2.5 + 2.6 + 2.7
                            --epic 2.8  # Runs Epic 2.0 + 2.1 + 2.2 + 2.3 + 2.4 + 2.5 + 2.6 + 2.7 + 2.8
                            --epic 2.9  # Runs all epics (2.0 → 2.1 → 2.2 → ... → 2.9)
  --skip-cleanup           Skip cleanup step and don't cleanup on exit
  --rebuild                Rebuild Docker images before starting services
  --help, -h               Show this help message

Examples:
  $0                      # Run all tests (2.0 → 2.1 → 2.2 → ... → 2.9)
  $0 --all                # Run all tests
  $0 --epic 2.0           # Run Epic 2.0 only (WireGuard Configuration Generation)
  $0 --epic 2.1           # Run Epic 2.0 + 2.1 (Configuration + VM Services)
  $0 --epic 2.2           # Run Epic 2.0 + 2.1 + 2.2 (Configuration + VM Services + Connection)
  $0 --epic 2.3           # Run Epic 2.0 + 2.1 + 2.2 + 2.3 (Configuration + VM Services + Connection + Coordination)
  $0 --epic 2.4           # Run Epic 2.0 + 2.1 + 2.2 + 2.3 + 2.4
  $0 --epic 2.5           # Run Epic 2.0 + 2.1 + 2.2 + 2.3 + 2.4 + 2.5
  $0 --epic 2.6           # Run Epic 2.0 + 2.1 + 2.2 + 2.3 + 2.4 + 2.5 + 2.6
  $0 --epic 2.7           # Run Epic 2.0 + 2.1 + 2.2 + 2.3 + 2.4 + 2.5 + 2.6 + 2.7
  $0 --epic 2.8           # Run Epic 2.0 + 2.1 + 2.2 + 2.3 + 2.4 + 2.5 + 2.6 + 2.7 + 2.8
  $0 --epic 2.9           # Run all epics (2.0 → 2.1 → 2.2 → ... → 2.9)
  $0 --skip-cleanup       # Run tests without cleanup
  $0 --rebuild            # Rebuild Docker images before starting services
  $0 --epic 2.3 --rebuild # Run up to Epic 2.3 with image rebuild

Epic Descriptions:
  2.0  WireGuard Configuration Generation
       (Generates WireGuard keys and configs - prerequisite for 2.1+)
  2.1  VM Services Startup and Initialization
       (Starts MinIO, Python AI Service, User VM API - prerequisite for 2.2+)
  2.2  VM Edge Status Monitoring & WireGuard Connection Management
       (Requires: 2.0, 2.1)
  2.3  Post-WireGuard Edge ↔ VM Coordination
       (Requires: 2.0, 2.1, 2.2)
  2.4  Snapshot Capture & Dataset Progress Fixes
       (Requires: 2.0, 2.1, 2.2, 2.3)
  2.5  Edge → VM Dataset Sync & Upload
       (Requires: 2.0, 2.1, 2.2, 2.3, 2.4)
  2.6  VM-Side Model Management for Training Readiness
       (Requires: 2.0, 2.1, 2.2, 2.3, 2.4, 2.5)
  2.7  Model Training Pipeline
       (Requires: 2.0, 2.1, 2.2, 2.3, 2.4, 2.5, 2.6)
  2.8  VM → Edge Trained Model Sync & Deployment
       (Requires: 2.0, 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 2.7)
  2.9  Edge-Side Event Detection & Processing
       (Requires: 2.0, 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 2.7, 2.8)

Environment Variables:
  VM_API              VM API URL (default: http://localhost:8280)
  EDGE_API            Edge API URL (default: http://localhost:8081)
  TRAINING_SERVICE    Training service URL (default: http://localhost:8000)
  CLEANUP_ON_EXIT     Cleanup on exit (default: true)

EOF
}
