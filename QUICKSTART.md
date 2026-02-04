# QUICKSTART

Get Ess-Queue-Ess running in under 5 minutes.

## Prerequisites

- Docker and Docker Compose (recommended)
- OR Go 1.23+ (for local development)

## Quick Start with Docker

1. **Clone and start**:
   ```bash
   git clone https://github.com/tonyellard/ess-queue-ess.git
   cd ess-queue-ess
   docker compose up -d
   ```

2. **Verify it's running**:
   ```bash
   curl http://localhost:9324/health
   # Should return: {"status":"healthy"}
   ```

3. **Open the Admin UI**:
   Open your browser to `http://localhost:9324/admin` to see the web interface with pre-configured queues from the example config.

4. **Create your first queue** (using AWS CLI):
   ```bash
   aws sqs create-queue --queue-name my-first-queue --endpoint-url http://localhost:9324
   ```

5. **Send a message**:
   ```bash
   aws sqs send-message \
     --queue-url http://localhost:9324/my-first-queue \
     --message-body "Hello, Ess-Queue-Ess!" \
     --endpoint-url http://localhost:9324
   ```

6. **Receive the message**:
   ```bash
   aws sqs receive-message \
     --queue-url http://localhost:9324/my-first-queue \
     --endpoint-url http://localhost:9324
   ```

7. **View in Admin UI**: Refresh the admin page to see your new queue and message!

## Quick Start with Go

1. **Clone, build, and run**:
   ```bash
   git clone https://github.com/tonyellard/ess-queue-ess.git
   cd ess-queue-ess
   make run
   ```

2. **Open Admin UI**: Visit `http://localhost:9324/admin` in your browser

3. **In another terminal, test the service**:
   ```bash
   # Create queue
   curl -X POST http://localhost:9324/ \
     -d "Action=CreateQueue&QueueName=test-queue"

   # Send message
   curl -X POST http://localhost:9324/ \
     -d "Action=SendMessage&QueueUrl=http://localhost:9324/test-queue&MessageBody=Hello"

   # Receive message
   curl -X POST http://localhost:9324/ \
     -d "Action=ReceiveMessage&QueueUrl=http://localhost:9324/test-queue&MaxNumberOfMessages=10"
   ```

## Bootstrap Queues with Configuration

1. **Create a config file**:
   ```bash
   make config  # Creates config.yaml from example
   ```

2. **Edit config.yaml** to define your queues:
   ```yaml
   queues:
     - name: "my-app-queue"
       visibility_timeout: 30
       message_retention_period: 345600
   ```

3. **Run with config**:
   ```bash
   make run-with-config
   # OR
   docker compose up -d  # Config is auto-mounted
   ```

Your queues will be created automatically on startup!

## Using with Your Application

### Python (boto3)

```python
import boto3

sqs = boto3.client('sqs',
    endpoint_url='http://localhost:9324',
    region_name='us-east-1',
    aws_access_key_id='test',
    aws_secret_access_key='test'
)

# Create queue
response = sqs.create_queue(QueueName='app-queue')
queue_url = response['QueueUrl']

# Send message
sqs.send_message(QueueUrl=queue_url, MessageBody='{"task": "process"}')

# Receive and process
messages = sqs.receive_message(QueueUrl=queue_url, MaxNumberOfMessages=10)
for msg in messages.get('Messages', []):
    print(f"Processing: {msg['Body']}")
    sqs.delete_message(QueueUrl=queue_url, ReceiptHandle=msg['ReceiptHandle'])
```

### JavaScript/Node.js (AWS SDK v3)

```javascript
import { SQSClient, CreateQueueCommand, SendMessageCommand, ReceiveMessageCommand } from "@aws-sdk/client-sqs";

const client = new SQSClient({
  endpoint: "http://localhost:9324",
  region: "us-east-1",
  credentials: { accessKeyId: "test", secretAccessKey: "test" }
});

// Create queue
const { QueueUrl } = await client.send(new CreateQueueCommand({ QueueName: "app-queue" }));

// Send message
await client.send(new SendMessageCommand({ QueueUrl, MessageBody: "Hello!" }));

// Receive messages
const { Messages } = await client.send(new ReceiveMessageCommand({ QueueUrl, MaxNumberOfMessages: 10 }));
```

### .NET (AWS SDK)

```csharp
using Amazon.SQS;
using Amazon.SQS.Model;

var config = new AmazonSQSConfig
{
    ServiceURL = "http://localhost:9324"
};
var client = new AmazonSQSClient(config);

// Create queue
var createResponse = await client.CreateQueueAsync("app-queue");
var queueUrl = createResponse.QueueUrl;

// Send message
await client.SendMessageAsync(queueUrl, "Hello from .NET!");

// Receive messages
var receiveResponse = await client.ReceiveMessageAsync(new ReceiveMessageRequest
{
    QueueUrl = queueUrl,
    MaxNumberOfMessages = 10
});
```

## Next Steps

- See [README.md](README.md) for full documentation
- Check supported operations and limitations
- View example .NET client in `dotnet-example/`

## Stopping the Service

```bash
# Docker
docker compose down

# Go (Ctrl+C in the terminal running the service)
```

That's it! You're ready to develop and test with Ess-Queue-Ess.
