#!/bin/bash
# Logging helper functions for test scripts

# Colors for output (must be set before sourcing this file)
# RED, GREEN, YELLOW, BLUE, NC should be defined

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_section() {
    echo -e "\n${BLUE}=== $1 ===${NC}\n"
}
