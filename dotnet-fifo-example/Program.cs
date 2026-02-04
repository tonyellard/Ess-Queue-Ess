using Amazon.SQS;
using Amazon.SQS.Model;
using System;
using System.Collections.Generic;
using System.Threading.Tasks;
using System.Text.Json;

namespace EssQueueEssFifoExample
{
    class Program
    {
        static async Task Main(string[] args)
        {
            Console.WriteLine("=== Ess-Queue-Ess FIFO & DLQ Example ===\n");

            var config = new AmazonSQSConfig
            {
                ServiceURL = "http://localhost:9324",
                AuthenticationRegion = "us-east-1"
            };

            using var client = new AmazonSQSClient(config);

            await TestFifoQueue(client);
            await TestDLQ(client);

            Console.WriteLine("\n=== Example completed successfully! ===");
        }

        static async Task TestFifoQueue(IAmazonSQS client)
        {
            Console.WriteLine("1. Testing FIFO Queue...");

            // Create FIFO queue
            var createRequest = new CreateQueueRequest
            {
                QueueName = "dotnet-test.fifo",
                Attributes = new Dictionary<string, string>
                {
                    { "FifoQueue", "true" },
                    { "ContentBasedDeduplication", "true" }
                }
            };

            var createResponse = await client.CreateQueueAsync(createRequest);
            var queueUrl = createResponse.QueueUrl;
            Console.WriteLine($"   Created FIFO queue: {queueUrl}");

            // Send messages with message groups
            Console.WriteLine("\n2. Sending messages to message groups...");
            for (int i = 1; i <= 3; i++)
            {
                var sendRequest = new SendMessageRequest
                {
                    QueueUrl = queueUrl,
                    MessageBody = $"Order {i} for Group A",
                    MessageGroupId = "group-A"
                };

                var sendResponse = await client.SendMessageAsync(sendRequest);
                Console.WriteLine($"   Sent to group-A: {sendResponse.MessageId.Substring(0, 8)}... (Seq: {sendResponse.SequenceNumber})");
            }

            for (int i = 1; i <= 2; i++)
            {
                var sendRequest = new SendMessageRequest
                {
                    QueueUrl = queueUrl,
                    MessageBody = $"Order {i} for Group B",
                    MessageGroupId = "group-B"
                };

                var sendResponse = await client.SendMessageAsync(sendRequest);
                Console.WriteLine($"   Sent to group-B: {sendResponse.MessageId.Substring(0, 8)}... (Seq: {sendResponse.SequenceNumber})");
            }

            // Test deduplication
            Console.WriteLine("\n3. Testing deduplication...");
            var dedupMsg = "Duplicate test";
            var msg1 = await client.SendMessageAsync(new SendMessageRequest
            {
                QueueUrl = queueUrl,
                MessageBody = dedupMsg,
                MessageGroupId = "group-C"
            });

            var msg2 = await client.SendMessageAsync(new SendMessageRequest
            {
                QueueUrl = queueUrl,
                MessageBody = dedupMsg,
                MessageGroupId = "group-C"
            });

            if (msg1.MessageId == msg2.MessageId)
            {
                Console.WriteLine("   ✓ Deduplication working: Same message ID returned");
            }
            else
            {
                Console.WriteLine("   ✗ Different message IDs (deduplication may not be enabled)");
            }

            // Receive messages
            Console.WriteLine("\n4. Receiving FIFO messages...");
            var receiveResponse = await client.ReceiveMessageAsync(new ReceiveMessageRequest
            {
                QueueUrl = queueUrl,
                MaxNumberOfMessages = 5
            });

            Console.WriteLine($"   Received {receiveResponse.Messages.Count} messages:");
            foreach (var msg in receiveResponse.Messages)
            {
                Console.WriteLine($"   - {msg.Body}");
                await client.DeleteMessageAsync(queueUrl, msg.ReceiptHandle);
            }

            // Cleanup
            await client.DeleteQueueAsync(queueUrl);
            Console.WriteLine("\n   ✓ Deleted FIFO queue");
        }

        static async Task TestDLQ(IAmazonSQS client)
        {
            Console.WriteLine("\n5. Testing Dead Letter Queue...");

            // Create DLQ
            var dlqResponse = await client.CreateQueueAsync("dotnet-dlq");
            var dlqUrl = dlqResponse.QueueUrl;
            Console.WriteLine($"   Created DLQ: {dlqUrl}");

            // Get DLQ ARN
            var dlqAttrs = await client.GetQueueAttributesAsync(new GetQueueAttributesRequest
            {
                QueueUrl = dlqUrl,
                AttributeNames = new List<string> { "QueueArn" }
            });
            var dlqArn = dlqAttrs.Attributes["QueueArn"];
            Console.WriteLine($"   DLQ ARN: {dlqArn}");

            // Create main queue with redrive policy
            var redrivePolicy = new
            {
                deadLetterTargetArn = dlqArn,
                maxReceiveCount = 3
            };

            var mainResponse = await client.CreateQueueAsync(new CreateQueueRequest
            {
                QueueName = "dotnet-main",
                Attributes = new Dictionary<string, string>
                {
                    { "RedrivePolicy", JsonSerializer.Serialize(redrivePolicy) }
                }
            });
            var mainUrl = mainResponse.QueueUrl;
            Console.WriteLine($"   Created main queue: {mainUrl}");
            Console.WriteLine($"   Max receive count: 3");

            // Send test message
            Console.WriteLine("\n6. Sending test message...");
            var testMsg = await client.SendMessageAsync(new SendMessageRequest
            {
                QueueUrl = mainUrl,
                MessageBody = "Test message for DLQ"
            });
            Console.WriteLine($"   Sent: {testMsg.MessageId.Substring(0, 8)}...");

            // Simulate failed processing
            Console.WriteLine("\n7. Simulating failed processing...");
            for (int i = 1; i <= 4; i++)
            {
                var recv = await client.ReceiveMessageAsync(new ReceiveMessageRequest
                {
                    QueueUrl = mainUrl,
                    MaxNumberOfMessages = 1,
                    VisibilityTimeout = 1
                });

                if (recv.Messages.Count > 0)
                {
                    Console.WriteLine($"   Attempt {i}: Received (not deleting)");
                    await Task.Delay(1500); // Wait for visibility timeout
                }
                else
                {
                    Console.WriteLine($"   Attempt {i}: No messages (may have moved to DLQ)");
                }
            }

            // Check DLQ
            Console.WriteLine("\n8. Checking DLQ...");
            var dlqMessages = await client.ReceiveMessageAsync(new ReceiveMessageRequest
            {
                QueueUrl = dlqUrl,
                MaxNumberOfMessages = 10
            });

            if (dlqMessages.Messages.Count > 0)
            {
                Console.WriteLine($"   ✓ Found {dlqMessages.Messages.Count} message(s) in DLQ:");
                foreach (var msg in dlqMessages.Messages)
                {
                    Console.WriteLine($"     - {msg.Body}");
                    await client.DeleteMessageAsync(dlqUrl, msg.ReceiptHandle);
                }
            }
            else
            {
                Console.WriteLine("   No messages in DLQ");
            }

            // Cleanup
            await client.DeleteQueueAsync(mainUrl);
            await client.DeleteQueueAsync(dlqUrl);
            Console.WriteLine("\n   ✓ Deleted test queues");
        }
    }
}
