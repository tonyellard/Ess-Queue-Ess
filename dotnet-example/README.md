# .NET Example for Ess-Queue-Ess

This example demonstrates how to use Ess-Queue-Ess with the AWS SDK for .NET.

## Prerequisites

- .NET 8.0 SDK or later
- Ess-Queue-Ess running on `http://localhost:9324`

## Running the Example

1. **Start Ess-Queue-Ess**:
   ```bash
   cd ..
   docker compose up -d
   # OR
   make run
   ```

2. **Run the .NET example**:
   ```bash
   cd dotnet-example
   dotnet run
   ```

## What This Example Does

1. Creates a queue named `dotnet-example-queue`
2. Lists all available queues
3. Sends 5 test messages to the queue
4. Gets queue attributes (message counts, etc.)
5. Receives the messages
6. Deletes each received message
7. Verifies the queue is empty
8. Deletes the queue

## Code Highlights

### Configure SQS Client

```csharp
var config = new AmazonSQSConfig
{
    ServiceURL = "http://localhost:9324",
    AuthenticationRegion = "us-east-1"
};
var client = new AmazonSQSClient(config);
```

### Send a Message

```csharp
var sendRequest = new SendMessageRequest
{
    QueueUrl = queueUrl,
    MessageBody = "Hello from .NET!",
    DelaySeconds = 0
};
await client.SendMessageAsync(sendRequest);
```

### Receive Messages

```csharp
var receiveRequest = new ReceiveMessageRequest
{
    QueueUrl = queueUrl,
    MaxNumberOfMessages = 10,
    VisibilityTimeout = 30
};
var response = await client.ReceiveMessageAsync(receiveRequest);
```

### Delete a Message

```csharp
await client.DeleteMessageAsync(new DeleteMessageRequest
{
    QueueUrl = queueUrl,
    ReceiptHandle = message.ReceiptHandle
});
```

## Expected Output

```
=== Ess-Queue-Ess .NET Example ===

1. Creating queue 'dotnet-example-queue'...
   Queue created: http://localhost:9324/dotnet-example-queue

2. Listing all queues...
   - http://localhost:9324/dotnet-example-queue

3. Sending messages...
   Sent message #1 - ID: <uuid>
   Sent message #2 - ID: <uuid>
   ...

4. Getting queue attributes...
   ApproximateNumberOfMessages: 5
   ...

5. Receiving messages...
   Received 5 messages:
   - Message ID: <uuid>
     Body: Test message #1 from .NET
   ...

6. Deleting messages...
   Deleted message: <uuid>
   ...

7. Verifying queue is empty...
   Messages remaining: 0

8. Deleting the queue...
   Queue deleted.

=== Example completed successfully! ===
```

## Integration with Your Application

To use Ess-Queue-Ess in your .NET application:

1. Add the AWS SQS SDK:
   ```bash
   dotnet add package AWSSDK.SQS
   ```

2. Configure the client to point to your local instance:
   ```csharp
   var config = new AmazonSQSConfig
   {
       ServiceURL = Environment.GetEnvironmentVariable("SQS_ENDPOINT") ?? "http://localhost:9324"
   };
   ```

3. Use the standard AWS SDK methods - no code changes needed!
