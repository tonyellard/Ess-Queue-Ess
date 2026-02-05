#!/bin/bash

# Test DLQ visibility timeout behavior
# Usage: ./test-dlq.sh <queue-name> <visibility-timeout>

QUEUE_NAME=${1:-test.fifo}
VISIBILITY_TIMEOUT=${2:-2}
ENDPOINT="http://localhost:9324"

echo "=== DLQ Visibility Timeout Test ==="
echo "Queue: $QUEUE_NAME"
echo "Visibility Timeout: ${VISIBILITY_TIMEOUT}s"
echo ""

# Send a test message
echo "[$(date +%H:%M:%S)] Sending test message..."
MESSAGE_ID=$(curl -s -X POST "$ENDPOINT/" \
  -H "Content-Type: application/x-amz-json-1.0" \
  -H "X-Amz-Target: AmazonSQS.SendMessage" \
  -d "{
    \"QueueUrl\": \"/$QUEUE_NAME\",
    \"MessageBody\": \"Test message at $(date)\",
    \"MessageGroupId\": \"test-group\",
    \"MessageDeduplicationId\": \"test-$(date +%s)\"
  }" | grep -o '"MessageId":"[^"]*"' | cut -d'"' -f4)

echo "Message sent: $MESSAGE_ID"
echo ""

# Function to receive messages
receive_message() {
  local vis_timeout=$1
  curl -s -X POST "$ENDPOINT/" \
    -H "Content-Type: application/x-amz-json-1.0" \
    -H "X-Amz-Target: AmazonSQS.ReceiveMessage" \
    -d "{
      \"QueueUrl\": \"/$QUEUE_NAME\",
      \"MaxNumberOfMessages\": 1,
      \"VisibilityTimeout\": $vis_timeout
    }"
}

# First receive
echo "[$(date +%H:%M:%S)] Receive #1 (visibility timeout: ${VISIBILITY_TIMEOUT}s)..."
RESPONSE=$(receive_message $VISIBILITY_TIMEOUT)
if echo "$RESPONSE" | grep -q "MessageId"; then
  echo "✓ Message received"
  FIRST_RECEIVE_TIME=$(date +%s)
else
  echo "✗ No message received"
  exit 1
fi
echo ""

# Wait and try second receive
sleep 1
echo "[$(date +%H:%M:%S)] Receive #2 (should be invisible for ${VISIBILITY_TIMEOUT}s)..."
RESPONSE=$(receive_message $VISIBILITY_TIMEOUT)
if echo "$RESPONSE" | grep -q "MessageId"; then
  echo "✗ ERROR: Message received (should be invisible!)"
  exit 1
else
  echo "✓ Message invisible (as expected)"
fi
echo ""

# Keep trying to receive until message becomes visible again
echo "Polling every second to see when message becomes visible again..."
echo ""

ATTEMPT=0
while [ $ATTEMPT -lt 60 ]; do
  CURRENT_TIME=$(date +%s)
  ELAPSED=$((CURRENT_TIME - FIRST_RECEIVE_TIME))
  
  RESPONSE=$(receive_message $VISIBILITY_TIMEOUT)
  
  if echo "$RESPONSE" | grep -q "MessageId"; then
    echo "[$(date +%H:%M:%S)] ✓ Message visible again after ${ELAPSED}s (Receive #3)"
    SECOND_RECEIVE_TIME=$(date +%s)
    break
  else
    echo "[$(date +%H:%M:%S)] Still invisible (${ELAPSED}s elapsed)..."
  fi
  
  sleep 1
  ATTEMPT=$((ATTEMPT + 1))
done

if [ -z "$SECOND_RECEIVE_TIME" ]; then
  echo ""
  echo "Message never became visible again (likely moved to DLQ)"
  exit 0
fi

echo ""
echo "Continuing to poll to see when message moves to DLQ..."
echo ""

# Keep trying to receive after second receive
sleep 1
ATTEMPT=0
while [ $ATTEMPT -lt 60 ]; do
  CURRENT_TIME=$(date +%s)
  ELAPSED=$((CURRENT_TIME - SECOND_RECEIVE_TIME))
  
  RESPONSE=$(receive_message $VISIBILITY_TIMEOUT)
  
  if echo "$RESPONSE" | grep -q "MessageId"; then
    echo "[$(date +%H:%M:%S)] Still in main queue after ${ELAPSED}s"
    sleep 1
  else
    echo "[$(date +%H:%M:%S)] ✓ Message no longer in queue after ${ELAPSED}s (moved to DLQ!)"
    break
  fi
  
  ATTEMPT=$((ATTEMPT + 1))
done

echo ""
echo "=== Test Complete ==="
