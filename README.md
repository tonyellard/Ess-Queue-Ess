# Ess-Queue-Ess

A lightweight, AWS SQS-compatible message queue emulator for local development and testing.

## Overview

Ess-Queue-Ess provides a local AWS Simple Queue Service (SQS) emulator that implements the core SQS API operations. Perfect for development, testing, and CI/CD pipelines without requiring AWS credentials or incurring AWS costs.

## Features

- **Core SQS Operations**: CreateQueue, DeleteQueue, ListQueues, SendMessage, ReceiveMessage, DeleteMessage
- **FIFO Queues**: First-In-First-Out queues with exactly-once processing and message ordering
- **Dead Letter Queues (DLQ)**: Automatic message movement to DLQ after max receive count
- **Message Redrive**: Move messages from DLQ back to source queue via StartMessageMoveTask API
- **Message Deduplication**: Content-based and explicit deduplication for FIFO queues
- **Message Groups**: Ordered message processing within groups for FIFO queues
- **Queue Attributes**: Get and monitor queue statistics (message counts, visibility settings)
- **Message Management**: Visibility timeout, message retention, delay delivery
- **Web Admin UI**: Browser-based interface to inspect queues and messages in real-time
- **Configuration Bootstrap**: Define queues in YAML config to auto-create on startup
- **Multi-SDK Support**: Compatible with AWS CLI, Python boto3, .NET AWS SDK, and other AWS SDKs
- **Dual Protocol Support**: Handles both JSON (AWS CLI v2) and Query (XML) protocols
- **Docker Support**: Run as a containerized service with docker-compose
- **Zero Configuration**: Works out of the box with sensible defaults
- **Lightweight**: Minimal dependencies, fast startup

## Limitations

This emulator is designed for local development and testing. It intentionally does not implement:

- **No Persistence**: Messages are stored in-memory only; they are lost on restart
- **No IAM/Authentication**: All requests are accepted without authentication
- **No Encryption**: Server-side encryption (SSE) not supported
- **Simplified Deduplication Window**: Fixed 5-minute window (configurable in real AWS SQS)
- **Simplified Message Attributes**: Basic support only
- **No Batch Operations**: SendMessageBatch and DeleteMessageBatch not yet implemented
- **Immediate Redrive**: Message move tasks complete immediately (no async processing)

## QUICKSTART

See [QUICKSTART.md](QUICKSTART.md) for a quick getting started guide.

## Documentation

- **[FIFO Queues and Dead Letter Queues](docs/FIFO_AND_DLQ.md)** - Complete guide to FIFO queues, DLQ, and message redrive
- **[Configuration Bootstrap](docs/CONFIGURATION.md)** - Auto-create queues from YAML config (if exists)
- **[Admin UI Guide](docs/ADMIN_UI.md)** - Web interface documentation (if exists)

## Installation

### Using Docker (Recommended)

```bash
git clone https://github.com/tonyellard/ess-queue-ess.git
cd ess-queue-ess
docker compose up -d
```

The service will be available at `http://localhost:9324`

### Using Go

```bash
git clone https://github.com/tonyellard/ess-queue-ess.git
cd ess-queue-ess
make build
make run
```

## Usage

### With AWS SDK (Python boto3)

```python
import boto3

# Configure boto3 to use local endpoint
sqs = boto3.client('sqs',
    endpoint_url='http://localhost:9324',
    region_name='us-east-1',
    aws_access_key_id='dummy',
    aws_secret_access_key='dummy'
)

# Create a queue
response = sqs.create_queue(QueueName='my-queue')
queue_url = response['QueueUrl']

# Send a message
sqs.send_message(
    QueueUrl=queue_url,
    MessageBody='Hello from Ess-Queue-Ess!'
)

# Receive messages
messages = sqs.receive_message(
    QueueUrl=queue_url,
    MaxNumberOfMessages=10
)

# Process and delete
for msg in messages.get('Messages', []):
    print(msg['Body'])
    sqs.delete_message(
        QueueUrl=queue_url,
        ReceiptHandle=msg['ReceiptHandle']
    )
```

### With AWS CLI

```bash
# Set dummy credentials (required even for local testing)
export AWS_ACCESS_KEY_ID=dummy
export AWS_SECRET_ACCESS_KEY=dummy
export AWS_DEFAULT_REGION=us-east-1

# Set endpoint
export AWS_ENDPOINT_URL=http://localhost:9324

# Create queue
aws sqs create-queue --queue-name test-queue

# Send message
aws sqs send-message --queue-url http://localhost:9324/test-queue --message-body "Test message"

# Receive messages
aws sqs receive-message --queue-url http://localhost:9324/test-queue

# Delete queue
aws sqs delete-queue --queue-url http://localhost:9324/test-queue
```

**Note**: The AWS CLI requires credentials even for local endpoints. Use dummy values as shown above.

## Admin Web Interface

Access the web-based admin UI to inspect and manage queues:

```
http://localhost:9324/admin
```

The admin interface provides:
- **Real-time queue statistics** (total, visible, in-flight, delayed messages)
- **Queue management**:
  - Create new queues with custom settings (visibility timeout, retention, message size)
  - Delete queues with confirmation dialog
  - Send test messages to any queue
  - Export current queue configuration as YAML
- **Queue inspection**:
  - List of all queues with message counts
  - Expandable view to inspect message contents
  - Auto-refresh every 5 seconds
  - Click any queue to view its messages

### Admin API Endpoints

The admin UI uses the following REST API endpoints (also available for programmatic access):

- `GET /admin/api/queues` - List all queues with messages
- `POST /admin/api/queue` - Create a new queue
- `DELETE /admin/api/queue?name={name}` - Delete a queue
- `POST /admin/api/message` - Send a test message to a queue
- `GET /admin/api/config/export` - Download current queue configuration as YAML

## Configuration

### Bootstrap Queues with YAML

Create queues automatically on startup using a configuration file:

1. **Create config.yaml** (or use the example):
   ```bash
   make config  # Creates config.yaml from config.example.yaml
   ```

2. **Edit config.yaml**:
   ```yaml
   server:
     port: 9324
     host: "0.0.0.0"

   queues:
     - name: "my-app-queue"
       visibility_timeout: 30
       message_retention_period: 345600  # 4 days
       maximum_message_size: 262144      # 256KB
       delay_seconds: 0
       receive_message_wait_time: 0
   ```

3. **Run with config**:
   ```bash
   # Using Go
   make run-with-config
   
   # Or directly
   ./ess-queue-ess --config config.yaml
   
   # Docker (config.yaml is auto-mounted)
   docker compose up -d
   ```

### Environment Variables

- `PORT`: Server port (default: 9324)

### Docker Compose

```yaml
services:
  ess-queue-ess:
    image: ess-queue-ess:latest
    ports:
      - "9324:9324"
    environment:
      - PORT=9324
    volumes:
      - ./config.yaml:/app/config.yaml:ro
    command: ["./ess-queue-ess", "--config", "/app/config.yaml"]
```

## Makefile Commands

```bash
make help              # Show all available commands
make build             # Build the Go binary
make run               # Run locally
make run-with-config   # Run with config.yaml
make config            # Create config.yaml from example
make test              # Run unit tests
make docker-build      # Build Docker image
make docker-run        # Start with docker-compose
make docker-stop       # Stop docker-compose
make docker-logs       # View container logs
```

## Supported SQS Operations

- ✅ CreateQueue
- ✅ DeleteQueue
- ✅ ListQueues
- ✅ SendMessage
- ✅ ReceiveMessage
- ✅ DeleteMessage
- ✅ GetQueueAttributes
- ✅ PurgeQueue

Not yet implemented:
- ⏳ SendMessageBatch
- ⏳ DeleteMessageBatch
- ⏳ ChangeMessageVisibility
- ⏳ SetQueueAttributes

## Development

### Project Structure

```
ess-queue-ess/
├── main.go           # HTTP server and routing
├── handlers.go       # SQS API request handlers
├── queue.go          # Queue and message data structures
├── Dockerfile        # Multi-stage Docker build
├── docker-compose.yml
├── Makefile
└── README.md
```

### Building from Source

```bash
# Clone the repository
git clone https://github.com/tonyellard/ess-queue-ess.git
cd ess-queue-ess

# Download dependencies
make deps

# Build
make build

# Run tests
make test

# Run locally
make run
```

## Support

[TBD] - Issue tracker and contribution guidelines coming soon.

## License

Licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE) for details.

**Trademark Notice**: Not affiliated with, endorsed by, or sponsored by Amazon Web Services (AWS). Amazon SQS is a trademark of Amazon.com, Inc. or its affiliates.
