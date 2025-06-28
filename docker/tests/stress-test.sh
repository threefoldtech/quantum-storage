#!/bin/bash

# Quantum Storage Stress Testing
# Tests system behavior under high load and concurrent failures

set -euo pipefail

source "$(dirname "$0")/test-framework.sh"

# Stress test: Concurrent operations with failures
test_concurrent_operations_with_failures() {
    log "=== STRESS TEST: Concurrent operations with failures ==="
    
    generate_test_data 20 10 "stress_test"
    start_container
    
    # Start multiple background operations
    (
        for i in {1..5}; do
            copy_test_data &
        done
        wait
    ) &
    local operations_pid=$!
    
    # Introduce random failures during operations
    sleep 2
    kill_zstor &
    sleep 1
    kill_zdb_backend 9903 &
    sleep 2
    kill_zdbfs &
    
    # Random restore timing
    sleep $((RANDOM % 3 + 1))
    restore_zstor &
    sleep $((RANDOM % 2 + 1))
    restore_zdb_backend 9903 &
    sleep $((RANDOM % 2 + 1))
    restore_zdbfs &
    
    # Wait for operations to complete
    wait $operations_pid || true
    
    sleep 10  # Give system time to stabilize
    verify_data_integrity "concurrent_operations_with_failures"
    
    cleanup
}

# Stress test: Rapid failure/recovery cycles
test_rapid_failure_recovery() {
    log "=== STRESS TEST: Rapid failure/recovery cycles ==="
    
    generate_test_data 15 5 "rapid_test"
    start_container
    copy_test_data
    
    # Rapid cycles of kill/restore
    for i in {1..3}; do
        log "Failure/recovery cycle $i"
        kill_zstor
        sleep 1
        restore_zstor
        sleep 2
        
        kill_zdb_backend 9901
        sleep 1
        restore_zdb_backend 9901
        sleep 2
    done
    
    sleep 5
    verify_data_integrity "rapid_failure_recovery"
    
    cleanup
}

# Stress test: Large file operations under failure
test_large_files_under_failure() {
    log "=== STRESS TEST: Large files under failure ==="
    
    generate_test_data 100 2 "large_test"  # 100MB files
    start_container
    
    # Start copying large files
    (
        sleep 3
        copy_test_data
    ) &
    local copy_pid=$!
    
    # Kill components during large file transfer
    sleep 5
    kill_zstor
    sleep 3
    kill_zdb_backend 9902
    
    # Wait a bit then restore
    sleep 5
    restore_zstor
    sleep 2
    restore_zdb_backend 9902
    
    # Wait for copy to complete
    wait $copy_pid || true
    
    sleep 10
    verify_data_integrity "large_files_under_failure"
    
    cleanup
}

# Run stress tests
run_stress_tests() {
    log "Starting Quantum Storage Stress Tests..."
    
    setup
    
    test_concurrent_operations_with_failures
    test_rapid_failure_recovery
    test_large_files_under_failure
    
    log "=== STRESS TEST SUMMARY ==="
    log "Tests passed: ${TESTS_PASSED}"
    log "Tests failed: ${TESTS_FAILED}"
    
    if [ "$TESTS_FAILED" -eq 0 ]; then
        log "🎉 All stress tests passed!"
        exit 0
    else
        error "❌ Some stress tests failed!"
        exit 1
    fi
}

# Main execution for stress tests
case "${1:-all}" in
    "concurrent")
        setup
        test_concurrent_operations_with_failures
        ;;
    "rapid")
        setup
        test_rapid_failure_recovery
        ;;
    "large")
        setup
        test_large_files_under_failure
        ;;
    "all"|*)
        run_stress_tests
        ;;
esac
