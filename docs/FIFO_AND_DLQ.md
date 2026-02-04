# FIFO Queues and Dead Letter Queue (DLQ) Support

## Overview

Ess-Queue-Ess now supports AWS SQS FIFO (First-In-First-Out) queues and Dead Letter Queues (DLQ) with redrive functionality.

## FIFO Queues

FIFO queues ensure exactly-once processing and preserve message ordering within message groups.

### Creating a FIFO Queue

FIFO queue names must end with `.fifo`:

```python
import boto3

sqs = boto3.client('sqs', endpoint_url='http://localhost:9324', region_name='us-east-1')

# Create FIFO queue
response = sqs.create_queue(
    QueueName='my-queue.fifo',
    Attributes={
        'FifoQueue': 'true',
        'ContentBasedDeduplication': 'true'  # Optional
    }
)
```

### FIFO Features

#### 1. Message Groups
Messages within the same group are processed in order:

```python
# Send messages to different groups
sqs.send_message(
    QueueUrl=queue_url,
    MessageBody='Order 1',
    MessageGroupId='group-A'
)

sqs.send_message(
    QueueUrl=queue_url,
    MessageBody='Order 2',
    MessageGroupId='group-B'
)
```

#### 2. Message Deduplication

**Content-Based Deduplication** (automatic):
```python
# Enable during queue creation
Attributes={'ContentBasedDeduplication': 'true'}

# Messages with identical bodies within 5 minutes are deduplicated
```

**Explicit Deduplication ID**:
```python
sqs.send_message(
    QueueUrl=queue_url,
    MessageBody='Important message',
    MessageGroupId='group-1',
    MessageDeduplicationId='unique-id-123'
)
```

#### 3. Sequence Numbers
Each message receives a unique sequence number for ordering:

```python
response = sqs.send_message(...)
sequence_number = response['SequenceNumber']
```

### FIFO Queue Behavior

- **Ordering**: Messages in the same group are delivered in the order sent
- **Deduplication Window**: 5 minutes (configurable in real AWS SQS)
- **Exactly-Once Processing**: Same message won't be delivered twice within deduplication window
- **Multiple Groups**: Messages from different groups can be processed in parallel

## Dead Letter Queues (DLQ)

DLQs capture messages that fail processing after a specified number of receive attempts.

### Creating a DLQ Setup

```python
# Step 1: Create the dead letter queue
dlq_response = sqs.create_queue(QueueName='my-dlq')
dlq_url = dlq_response['QueueUrl']

# Step 2: Get DLQ ARN
dlq_attrs = sqs.get_queue_attributes(
    QueueUrl=dlq_url,
    AttributeNames=['QueueArn']
)
dlq_arn = dlq_attrs['Attributes']['QueueArn']

# Step 3: Create main queue with redrive policy
import json

redrive_policy = {
    'deadLetterTargetArn': dlq_arn,
    'maxReceiveCount': 3  # Move to DLQ after 3 failed receives
}

main_response = sqs.create_queue(
    QueueName='my-main-queue',
    Attributes={
        'RedrivePolicy': json.dumps(redrive_policy)
    }
)
```

### How DLQ Works

1. **Normal Processing**: Messages are received and processed from the main queue
2. **Failed Processing**: If a message is received but not deleted (processing failed)
3. **Receive Count**: Each receive increments the message's receive count
4. **Auto-Move**: When receive count reaches `maxReceiveCount`, message moves to DLQ
5. **Analysis**: Inspect failed messages in DLQ to debug issues

### Example: Processing with DLQ

```python
# Receive message
response = sqs.receive_message(QueueUrl=main_queue_url)

if 'Messages' in response:
    for message in response['Messages']:
        try:
            # Process message
            process_message(message['Body'])
            
            # Delete on success
            sqs.delete_message(
                QueueUrl=main_queue_url,
                ReceiptHandle=message['ReceiptHandle']
            )
        except Exception as e:
            # Don't delete - let it go back to queue
            # After maxReceiveCount attempts, moves to DLQ
            print(f"Processing failed: {e}")
```

## Message Redrive

Move messages from DLQ back to the source queue after fixing issues.

### Using StartMessageMoveTask

```python
# Move messages from DLQ back to source queue
response = sqs.start_message_move_task(
    SourceArn=dlq_arn,
    DestinationArn=source_queue_arn,
    MaxNumberOfMessagesPerSecond=10  # Optional rate limit
)

task_handle = response['TaskHandle']
```

### Redrive API Operations

- **StartMessageMoveTask**: Begin moving messages from DLQ to source
- **ListMessageMoveTasks**: Check status of active redrive tasks
- **CancelMessageMoveTask**: Stop an in-progress redrive

> **Note**: In ess-queue-ess, redrive operations complete immediately for simplicity.

## FIFO with DLQ

FIFO queues can also use DLQ functionality:

```python
# Create FIFO DLQ
dlq_response = sqs.create_queue(
    QueueName='my-dlq.fifo',
    Attributes={'FifoQueue': 'true'}
)

# Create FIFO main queue with DLQ
main_response = sqs.create_queue(
    QueueName='my-queue.fifo',
    Attributes={
        'FifoQueue': 'true',
        'ContentBasedDeduplication': 'true',
        'RedrivePolicy': json.dumps({
            'deadLetterTargetArn': dlq_arn,
            'maxReceiveCount': 3
        })
    }
)
```

**Important**: FIFO main queues must use FIFO DLQs (and vice versa).

## Testing

Run the comprehensive test suite:

```bash
# Make sure ess-queue-ess is running
docker compose up -d

# Run FIFO and DLQ tests
python3 test/fifo_dlq_test.py
```

## Attributes Reference

### FIFO Queue Attributes
- `FifoQueue`: `"true"` - Enables FIFO queue behavior
- `ContentBasedDeduplication`: `"true"` - Auto-deduplicate based on message body

### DLQ Attributes
- `RedrivePolicy`: JSON string with:
  - `deadLetterTargetArn`: ARN of the DLQ
  - `maxReceiveCount`: Number of receives before moving to DLQ

### Message Parameters (FIFO)
- `MessageGroupId`: Required for FIFO queues - defines ordering group
- `MessageDeduplicationId`: Optional - explicit deduplication ID
- `SequenceNumber`: Returned in response - indicates message order

## Use Cases

### FIFO Queues
- **Order Processing**: Ensure orders are processed in sequence
- **Transaction Logs**: Maintain chronological order of events
- **Chat Messages**: Preserve conversation order
- **State Machines**: Sequential state transitions

### Dead Letter Queues
- **Error Analysis**: Capture messages that fail processing
- **Debugging**: Inspect problematic messages
- **Alerting**: Monitor DLQ depth to detect issues
- **Retry Logic**: Reprocess failed messages after fixing bugs

## Limitations

Current ess-queue-ess implementation:
- Deduplication window: 5 minutes (fixed)
- Message move tasks complete immediately (no async processing)
- In-memory storage only (not persistent)
- Single server (no clustering)

## CLI Examples

### AWS CLI with FIFO Queue

```bash
# Create FIFO queue
aws sqs create-queue \
  --queue-name my-queue.fifo \
  --attributes FifoQueue=true,ContentBasedDeduplication=true \
  --endpoint-url http://localhost:9324

# Send message to FIFO queue
aws sqs send-message \
  --queue-url http://localhost:9324/my-queue.fifo \
  --message-body "Test message" \
  --message-group-id "group-1" \
  --endpoint-url http://localhost:9324

# Receive from FIFO queue
aws sqs receive-message \
  --queue-url http://localhost:9324/my-queue.fifo \
  --endpoint-url http://localhost:9324
```

### AWS CLI with DLQ

```bash
# Create DLQ
aws sqs create-queue \
  --queue-name my-dlq \
  --endpoint-url http://localhost:9324

# Create main queue with redrive policy
aws sqs create-queue \
  --queue-name my-main-queue \
  --attributes 'RedrivePolicy="{\"deadLetterTargetArn\":\"arn:aws:sqs:us-east-1:000000000000:my-dlq\",\"maxReceiveCount\":3}"' \
  --endpoint-url http://localhost:9324
```

## Architecture

### FIFO Queue Components
```
Message → Deduplication Check → Group Assignment → Ordered Storage
                ↓                        ↓
         (5min cache)          (Group-based queues)
```

### DLQ Flow
```
Main Queue → Receive (attempt 1) → Failed Processing
          → Receive (attempt 2) → Failed Processing  
          → Receive (attempt 3) → Failed Processing
          → Auto-move to DLQ ✓

DLQ → Analyze/Debug → Fix Issue → Redrive to Main Queue
```

## Best Practices

1. **FIFO Queues**:
   - Use message groups to enable parallel processing
   - Keep message groups balanced for better throughput
   - Enable content-based deduplication when message bodies are unique

2. **Dead Letter Queues**:
   - Set `maxReceiveCount` based on retry tolerance (typical: 3-5)
   - Monitor DLQ depth with CloudWatch/metrics
   - Implement alerts when DLQ receives messages
   - Periodically review and redrive or delete DLQ messages

3. **Combining FIFO + DLQ**:
   - Ensure both queues are FIFO type
   - Use same message group IDs for redriven messages
   - Consider separate DLQs per message group for better isolation
