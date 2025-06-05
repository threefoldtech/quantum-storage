#!/bin/bash

# Quantum Storage Failure Testing Framework
# Tests data integrity under various component failure scenarios

set -euo pipefail

# Configuration
CONTAINER_NAME="qsfs-test"
IMAGE_NAME="qsfs"
TEST_DATA_DIR="/tmp/qsfs-test-data"
MOUNT_POINT="/mnt"
LOG_DIR="/tmp/qsfs-test-logs"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Test results
TESTS_PASSED=0
TESTS_FAILED=0

log() {
    echo -e "${GREEN}[$(date '+%Y-%m-%d %H:%M:%S')] $1${NC}"
}

warn() {
    echo -e "${YELLOW}[$(date '+%Y-%m-%d %H:%M:%S')] WARNING: $1${NC}"
}

error() {
    echo -e "${RED}[$(date '+%Y-%m-%d %H:%M:%S')] ERROR: $1${NC}"
}

# Cleanup function
cleanup() {
    log "Cleaning up test environment..."
    docker stop "${CONTAINER_NAME}" 2>/dev/null || true
    docker rm "${CONTAINER_NAME}" 2>/dev/null || true
    rm -rf "${TEST_DATA_DIR}" "${LOG_DIR}"
}

# Setup test environment
setup() {
    log "Setting up test environment..."
    mkdir -p "${TEST_DATA_DIR}" "${LOG_DIR}"

    # Build the image if it doesn't exist
    if ! docker image inspect "${IMAGE_NAME}" >/dev/null 2>&1; then
        log "Building quantum storage image..."
        cd "$(dirname "$0")/.."
        docker buildx build -t "${IMAGE_NAME}" .
        cd - >/dev/null
    fi
}

# Generate test data with checksums
generate_test_data() {
    local size_mb=$1
    local file_count=${2:-1}
    local prefix="${3:-testfile}"

    log "Generating ${file_count} test files of ${size_mb}MB each..."

    for i in $(seq 1 "$file_count"); do
        local filename="${TEST_DATA_DIR}/${prefix}_${i}.dat"
        dd if=/dev/urandom of="$filename" bs=1M count="$size_mb" 2>/dev/null
        sha256sum "$filename" > "${filename}.sha256"
    done
}

# Start quantum storage container
start_container() {
    log "Starting quantum storage container..."
    docker run -d \
        --cap-add SYS_ADMIN \
        --device /dev/fuse \
        --name "${CONTAINER_NAME}" \
        --tmpfs /data:exec,size=1G \
        "${IMAGE_NAME}"

    # Wait for system to be ready
    log "Waiting for quantum storage system to initialize..."
    local max_attempts=30
    local attempt=0

    while [ $attempt -lt $max_attempts ]; do
        if docker exec "${CONTAINER_NAME}" test -d "${MOUNT_POINT}" 2>/dev/null; then
            if docker exec "${CONTAINER_NAME}" ls "${MOUNT_POINT}" >/dev/null 2>&1; then
                log "Quantum storage system is ready!"
                return 0
            fi
        fi
        sleep 2
        ((attempt++))
    done

    error "Quantum storage system failed to initialize within ${max_attempts} attempts"
    return 1
}

# Copy test data to quantum storage
copy_test_data() {
    log "Copying test data to quantum storage..."
    for file in "${TEST_DATA_DIR}"/*.dat; do
        if [ -f "$file" ]; then
            local basename=$(basename "$file")
            docker cp "$file" "${CONTAINER_NAME}:${MOUNT_POINT}/${basename}"
        fi
    done
}

# Verify data integrity
verify_data_integrity() {
    local test_name="$1"
    log "Verifying data integrity for test: ${test_name}..."

    local integrity_ok=true

    for checksum_file in "${TEST_DATA_DIR}"/*.sha256; do
        if [ -f "$checksum_file" ]; then
            local basename=$(basename "$checksum_file" .sha256)
            local expected_hash=$(cat "$checksum_file" | cut -d' ' -f1)

            # Get file from container and calculate hash
            docker cp "${CONTAINER_NAME}:${MOUNT_POINT}/${basename}" "${TEST_DATA_DIR}/retrieved_${basename}" 2>/dev/null || {
                error "Failed to retrieve file: ${basename}"
                integrity_ok=false
                continue
            }

            local actual_hash=$(sha256sum "${TEST_DATA_DIR}/retrieved_${basename}" | cut -d' ' -f1)

            if [ "$expected_hash" = "$actual_hash" ]; then
                log "✓ File ${basename} integrity verified"
            else
                error "✗ File ${basename} integrity check FAILED!"
                error "  Expected: $expected_hash"
                error "  Actual:   $actual_hash"
                integrity_ok=false
            fi

            rm -f "${TEST_DATA_DIR}/retrieved_${basename}"
        fi
    done

    if [ "$integrity_ok" = true ]; then
        log "✓ All files passed integrity check for test: ${test_name}"
        ((TESTS_PASSED++))
        return 0
    else
        error "✗ Data integrity test FAILED for: ${test_name}"
        ((TESTS_FAILED++))
        return 1
    fi
}

# Simulate component failures
kill_zstor() {
    log "Killing zstor process..."
    docker exec "${CONTAINER_NAME}" pkill -f zstor || true
}

kill_zdb_backend() {
    local port="$1"
    log "Killing ZDB backend on port ${port}..."
    docker exec "${CONTAINER_NAME}" pkill -f "zdb.*--port ${port}" || true
}

kill_zdbfs() {
    log "Killing zdbfs process..."
    docker exec "${CONTAINER_NAME}" pkill -f zdbfs || true
}

kill_frontend_zdb() {
    log "Killing frontend ZDB..."
    docker exec "${CONTAINER_NAME}" pkill -f "zdb.*--index /data/index" || true
}

# Restore components
restore_zstor() {
    log "Restoring zstor..."
    docker exec "${CONTAINER_NAME}" zstor -c /etc/zstor-default.toml --log_file /var/log/zstor.log monitor &
    sleep 3
}

restore_zdb_backend() {
    local port="$1"
    local data_num="${port: -1}"  # Extract last digit
    log "Restoring ZDB backend on port ${port}..."
    docker exec "${CONTAINER_NAME}" zdb --port "$port" --data "/data/data${data_num}" --index "/data/index${data_num}" --background --logfile "/var/log/zdb${data_num}.log"
    sleep 2

    # Recreate namespaces
    docker exec "${CONTAINER_NAME}" redis-cli -p "$port" NSNEW "data${data_num}" || true
    docker exec "${CONTAINER_NAME}" redis-cli -p "$port" NSSET "data${data_num}" password zdbpassword || true
    docker exec "${CONTAINER_NAME}" redis-cli -p "$port" NSSET "data${data_num}" mode seq || true
    docker exec "${CONTAINER_NAME}" redis-cli -p "$port" NSNEW "meta${data_num}" || true
    docker exec "${CONTAINER_NAME}" redis-cli -p "$port" NSSET "meta${data_num}" password zdbpassword || true
}

restore_zdbfs() {
    log "Restoring zdbfs..."
    docker exec "${CONTAINER_NAME}" zdbfs -o autons -o background /mnt &
    sleep 3
}

restore_frontend_zdb() {
    log "Restoring frontend ZDB..."
    docker exec "${CONTAINER_NAME}" zdb \
        --index /data/index \
        --data /data/data \
        --logfile /var/log/zdb.log \
        --datasize 67108864 \
        --hook /usr/local/bin/zdb-hook.sh \
        --rotate 1 \
        --background &
    sleep 3
}

# Test scenario: Kill zstor during data upload
test_zstor_failure_during_upload() {
    log "=== TEST: zstor failure during data upload ==="

    generate_test_data 100 3 "zstor_test"
    start_container

    # Start copying data in background
    (
        sleep 2
        copy_test_data
    ) &
    local copy_pid=$!

    # Kill zstor after 1 second
    sleep 1
    kill_zstor

    # Wait a bit then restore
    sleep 3
    restore_zstor

    # Wait for copy to complete
    wait $copy_pid || true

    sleep 5
    verify_data_integrity "zstor_failure_during_upload"

    cleanup
}

# Test scenario: Kill backend ZDB during operation
test_backend_zdb_failure() {
    log "=== TEST: Backend ZDB failure ==="

    generate_test_data 5 2 "backend_test"
    start_container
    copy_test_data

    # Kill one backend ZDB
    kill_zdb_backend 9901

    # Try to access data
    sleep 2

    # Restore the backend
    restore_zdb_backend 9901

    sleep 3
    verify_data_integrity "backend_zdb_failure"

    cleanup
}

# Test scenario: Kill zdbfs during operation
test_zdbfs_failure() {
    log "=== TEST: zdbfs failure ==="

    generate_test_data 8 2 "zdbfs_test"
    start_container
    copy_test_data

    # Kill zdbfs
    kill_zdbfs

    sleep 2

    # Restore zdbfs
    restore_zdbfs

    sleep 3
    verify_data_integrity "zdbfs_failure"

    cleanup
}

# Test scenario: Multiple component failures
test_multiple_failures() {
    log "=== TEST: Multiple component failures ==="

    generate_test_data 15 4 "multi_test"
    start_container
    copy_test_data

    # Sequential failures
    kill_zstor
    sleep 1
    kill_zdb_backend 9902
    sleep 1
    kill_zdbfs

    sleep 3

    # Restore in order
    restore_zdb_backend 9902
    sleep 2
    restore_zstor
    sleep 2
    restore_zdbfs

    sleep 5
    verify_data_integrity "multiple_failures"

    cleanup
}

# Run all tests
run_all_tests() {
    log "Starting Quantum Storage Failure Tests..."

    setup

    test_zstor_failure_during_upload
    test_backend_zdb_failure
    test_zdbfs_failure
    test_multiple_failures

    log "=== TEST SUMMARY ==="
    log "Tests passed: ${TESTS_PASSED}"
    log "Tests failed: ${TESTS_FAILED}"

    if [ "$TESTS_FAILED" -eq 0 ]; then
        log "🎉 All tests passed!"
        exit 0
    else
        error "❌ Some tests failed!"
        exit 1
    fi
}

# Handle cleanup on script exit
trap cleanup EXIT

# Main execution
case "${1:-all}" in
    "zstor")
        setup
        test_zstor_failure_during_upload
        ;;
    "backend")
        setup
        test_backend_zdb_failure
        ;;
    "zdbfs")
        setup
        test_zdbfs_failure
        ;;
    "multi")
        setup
        test_multiple_failures
        ;;
    "all"|*)
        run_all_tests
        ;;
esac
