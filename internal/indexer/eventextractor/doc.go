// Package eventextractor provides a supplementary extractor that detects
// message queue producer and consumer patterns across Go, TypeScript, Python,
// and Java source code.
//
// Unlike primary language extractors that handle all symbols in a file, the
// event extractor focuses specifically on detecting messaging API usage patterns
// (Kafka, NATS, SQS, RabbitMQ/AMQP) and producing topic nodes with
// "publishes"/"subscribes" edges.
//
// Supported patterns:
//
// Go: sarama (Kafka), nats, sqs, amqp
// TypeScript/JavaScript: kafkajs, NestJS decorators (@MessagePattern, @EventPattern)
// Python: kafka-python, pika (RabbitMQ)
// Java: Spring @KafkaListener, KafkaTemplate, JMS
//
// This extractor is designed to run alongside the primary language extractor
// via FindAllExtractors, not as a replacement. It uses tree-sitter for AST
// parsing with the same grammar libraries used by other extractors in the
// indexer.
package eventextractor
