# Ess-Queue-Ess Tests

Integration tests for the Ess-Queue-Ess SQS emulator.

## Running Tests

### Prerequisites

- Ess-Queue-Ess running on `http://localhost:9324`
- Python 3 with `requests` library

### Start the Service

```bash
# Using Docker
docker compose up -d

# OR using Go
make run
```

### Run Integration Tests

```bash
# From project root
python3 test/integration_test.py

# OR using make
make integration-test
```

## Test Coverage

The integration tests verify:

### Core SQS Operations
- ✅ CreateQueue - Create new queues
- ✅ DeleteQueue - Remove queues
- ✅ ListQueues - List all available queues
- ✅ SendMessage - Send messages to queue
- ✅ ReceiveMessage - Receive messages from queue
- ✅ DeleteMessage - Delete specific message by receipt handle
- ✅ GetQueueAttributes - Get queue statistics
- ✅ PurgeQueue - Remove all messages from queue

### Admin Interface
- ✅ Health Check endpoint (`/health`)
- ✅ Admin UI accessibility (`/admin`)
- ✅ Admin API endpoint (`/admin/api/queues`)
- ✅ Real-time queue statistics
- ✅ Message visibility in admin API

### Message Flow
- ✅ Multiple message sending (batch testing)
- ✅ Message receive with max count
- ✅ Message visibility and receipt handles
- ✅ Queue purge functionality

## Expected Output

```
============================================================
  Ess-Queue-Ess Integration Tests
============================================================

▶ Testing: Health Check
  ✓ Health endpoint returns healthy status

▶ Testing: Admin UI
  ✓ Admin UI loads successfully

[... more tests ...]

============================================================
  ✓ All tests passed!
============================================================
```

## Troubleshooting

### Connection Refused

If you see: `Cannot connect to Ess-Queue-Ess`

**Solution**: Ensure the service is running:
```bash
docker compose up -d
# OR
make run
```

### Tests Fail After Docker Restart

**Solution**: Restart the container:
```bash
docker compose down
docker compose up -d
```

## Adding New Tests

To add new test cases:

1. Add a new test function in `integration_test.py`:
   ```python
   def test_my_feature():
       print_test("My Feature")
       # Your test code
       assert condition, "Error message"
       print_success("Feature works!")
   ```

2. Call it from `run_all_tests()`:
   ```python
   test_my_feature()
   ```

3. Run tests to verify:
   ```bash
   python3 test/integration_test.py
   ```
