// SPDX-License-Identifier: Apache-2.0

using Amazon.SQS;
using Amazon.SQS.Model;

namespace EssQueueEssExample;

class Program
{
    static async Task Main(string[] args)
    {
        // Configure SQS client to point to local Ess-Queue-Ess instance
        var config = new AmazonSQSConfig
        {
            ServiceURL = "http://localhost:9324",
            AuthenticationRegion = "us-east-1"
        };
        
        var client = new AmazonSQSClient(config);
        
        Console.WriteLine("=== Ess-Queue-Ess .NET Example ===\n");

        // 1. Create a queue
        Console.WriteLine("1. Creating queue 'dotnet-example-queue'...");
        var createQueueRequest = new CreateQueueRequest
        {
            QueueName = "dotnet-example-queue"
        };
        var createQueueResponse = await client.CreateQueueAsync(createQueueRequest);
        var queueUrl = createQueueResponse.QueueUrl;
        Console.WriteLine($"   Queue created: {queueUrl}\n");

        // 2. List queues
        Console.WriteLine("2. Listing all queues...");
        var listQueuesResponse = await client.ListQueuesAsync(new ListQueuesRequest());
        foreach (var url in listQueuesResponse.QueueUrls)
        {
            Console.WriteLine($"   - {url}");
        }
        Console.WriteLine();

        // 3. Send messages
        Console.WriteLine("3. Sending messages...");
        for (int i = 1; i <= 5; i++)
        {
            var sendMessageRequest = new SendMessageRequest
            {
                QueueUrl = queueUrl,
                MessageBody = $"Test message #{i} from .NET",
                DelaySeconds = 0
            };
            var sendResponse = await client.SendMessageAsync(sendMessageRequest);
            Console.WriteLine($"   Sent message #{i} - ID: {sendResponse.MessageId}");
        }
        Console.WriteLine();

        // 4. Get queue attributes
        Console.WriteLine("4. Getting queue attributes...");
        var attributesRequest = new GetQueueAttributesRequest
        {
            QueueUrl = queueUrl,
            AttributeNames = new List<string> { "All" }
        };
        var attributesResponse = await client.GetQueueAttributesAsync(attributesRequest);
        foreach (var attr in attributesResponse.Attributes)
        {
            Console.WriteLine($"   {attr.Key}: {attr.Value}");
        }
        Console.WriteLine();

        // 5. Receive messages
        Console.WriteLine("5. Receiving messages...");
        var receiveRequest = new ReceiveMessageRequest
        {
            QueueUrl = queueUrl,
            MaxNumberOfMessages = 10,
            VisibilityTimeout = 30,
            WaitTimeSeconds = 0
        };
        var receiveResponse = await client.ReceiveMessageAsync(receiveRequest);
        
        Console.WriteLine($"   Received {receiveResponse.Messages.Count} messages:");
        foreach (var message in receiveResponse.Messages)
        {
            Console.WriteLine($"   - Message ID: {message.MessageId}");
            Console.WriteLine($"     Body: {message.Body}");
            Console.WriteLine($"     Receipt Handle: {message.ReceiptHandle}");
        }
        Console.WriteLine();

        // 6. Delete messages
        Console.WriteLine("6. Deleting messages...");
        foreach (var message in receiveResponse.Messages)
        {
            var deleteRequest = new DeleteMessageRequest
            {
                QueueUrl = queueUrl,
                ReceiptHandle = message.ReceiptHandle
            };
            await client.DeleteMessageAsync(deleteRequest);
            Console.WriteLine($"   Deleted message: {message.MessageId}");
        }
        Console.WriteLine();

        // 7. Verify queue is empty
        Console.WriteLine("7. Verifying queue is empty...");
        var verifyReceive = await client.ReceiveMessageAsync(new ReceiveMessageRequest
        {
            QueueUrl = queueUrl,
            MaxNumberOfMessages = 10
        });
        Console.WriteLine($"   Messages remaining: {verifyReceive.Messages.Count}\n");

        // 8. Delete the queue
        Console.WriteLine("8. Deleting the queue...");
        await client.DeleteQueueAsync(new DeleteQueueRequest { QueueUrl = queueUrl });
        Console.WriteLine("   Queue deleted.\n");

        Console.WriteLine("=== Example completed successfully! ===");
    }
}
