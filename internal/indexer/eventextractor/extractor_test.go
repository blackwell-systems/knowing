package eventextractor

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

func newTestOpts(filePath string, content string) types.ExtractOptions {
	return types.ExtractOptions{
		RepoURL:  "github.com/test/repo",
		FilePath: filePath,
		FileHash: types.NewHash([]byte("test-file")),
		Content:  []byte(content),
	}
}

func TestEventExtractor_Name(t *testing.T) {
	e := NewEventExtractor()
	if got := e.Name(); got != "event-mq" {
		t.Errorf("Name() = %q, want %q", got, "event-mq")
	}
}

func TestEventExtractor_CanHandle(t *testing.T) {
	e := NewEventExtractor()

	tests := []struct {
		path string
		want bool
	}{
		{"main.go", true},
		{"src/app.ts", true},
		{"src/app.tsx", true},
		{"src/app.js", true},
		{"src/app.jsx", true},
		{"app.py", true},
		{"App.java", true},
		{"README.md", false},
		{"schema.yaml", false},
		{"node_modules/kafka/index.js", false},
		{"src/node_modules/lib/index.ts", false},
		{"data.json", false},
	}

	for _, tt := range tests {
		if got := e.CanHandle(tt.path); got != tt.want {
			t.Errorf("CanHandle(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestEventExtractor_GoKafkaProducer(t *testing.T) {
	e := NewEventExtractor()
	src := `package main

import "github.com/Shopify/sarama"

func publishOrder(producer sarama.SyncProducer) {
	msg := &sarama.ProducerMessage{
		Topic: "order-events",
		Value: sarama.StringEncoder("order created"),
	}
	producer.SendMessage(msg)
}
`
	opts := newTestOpts("cmd/publisher/main.go", src)
	result, err := e.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	// Should find at least one topic node
	var topicNodes []types.Node
	for _, n := range result.Nodes {
		if n.Kind == "topic" {
			topicNodes = append(topicNodes, n)
		}
	}
	if len(topicNodes) == 0 {
		t.Fatal("expected at least one topic node, got 0")
	}

	foundTopic := false
	for _, n := range topicNodes {
		if contains(n.QualifiedName, "order-events") {
			foundTopic = true
		}
	}
	if !foundTopic {
		t.Error("expected topic node with 'order-events' in qualified name")
	}

	// Should have a publishes edge
	foundPublishes := false
	for _, edge := range result.Edges {
		if edge.EdgeType == "publishes" {
			foundPublishes = true
		}
	}
	if !foundPublishes {
		t.Error("expected a 'publishes' edge")
	}
}

func TestEventExtractor_GoNatsSubscribe(t *testing.T) {
	e := NewEventExtractor()
	src := `package main

import "github.com/nats-io/nats.go"

func handleMessages(nc *nats.Conn) {
	nc.Subscribe("user.created", func(msg *nats.Msg) {
		// handle message
	})
}
`
	opts := newTestOpts("cmd/subscriber/main.go", src)
	result, err := e.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	var topicNodes []types.Node
	for _, n := range result.Nodes {
		if n.Kind == "topic" {
			topicNodes = append(topicNodes, n)
		}
	}
	if len(topicNodes) == 0 {
		t.Fatal("expected at least one topic node, got 0")
	}

	foundTopic := false
	for _, n := range topicNodes {
		if contains(n.QualifiedName, "user.created") {
			foundTopic = true
		}
	}
	if !foundTopic {
		t.Error("expected topic node with 'user.created' in qualified name")
	}

	// Should have a subscribes edge
	foundSubscribes := false
	for _, edge := range result.Edges {
		if edge.EdgeType == "subscribes" {
			foundSubscribes = true
		}
	}
	if !foundSubscribes {
		t.Error("expected a 'subscribes' edge")
	}
}

func TestEventExtractor_TypeScriptKafkaJS(t *testing.T) {
	e := NewEventExtractor()
	src := `import { Kafka } from 'kafkajs';

const kafka = new Kafka({ brokers: ['localhost:9092'] });
const producer = kafka.producer();

async function publishEvent() {
  await producer.send({
    topic: "payment-processed",
    messages: [{ value: 'done' }],
  });
}
`
	opts := newTestOpts("src/kafka/producer.ts", src)
	result, err := e.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	var topicNodes []types.Node
	for _, n := range result.Nodes {
		if n.Kind == "topic" {
			topicNodes = append(topicNodes, n)
		}
	}
	if len(topicNodes) == 0 {
		t.Fatal("expected at least one topic node, got 0")
	}

	foundTopic := false
	for _, n := range topicNodes {
		if contains(n.QualifiedName, "payment-processed") {
			foundTopic = true
		}
	}
	if !foundTopic {
		t.Errorf("expected topic node with 'payment-processed', got nodes: %v", topicNodes)
	}

	foundPublishes := false
	for _, edge := range result.Edges {
		if edge.EdgeType == "publishes" {
			foundPublishes = true
		}
	}
	if !foundPublishes {
		t.Error("expected a 'publishes' edge")
	}
}

func TestEventExtractor_PythonKafkaConsumer(t *testing.T) {
	e := NewEventExtractor()
	src := `from kafka import KafkaConsumer

def consume_events():
    consumer = KafkaConsumer("inventory-updates", bootstrap_servers=['localhost:9092'])
    for msg in consumer:
        process(msg)
`
	opts := newTestOpts("workers/consumer.py", src)
	result, err := e.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	var topicNodes []types.Node
	for _, n := range result.Nodes {
		if n.Kind == "topic" {
			topicNodes = append(topicNodes, n)
		}
	}
	if len(topicNodes) == 0 {
		t.Fatal("expected at least one topic node, got 0")
	}

	foundTopic := false
	for _, n := range topicNodes {
		if contains(n.QualifiedName, "inventory-updates") {
			foundTopic = true
		}
	}
	if !foundTopic {
		t.Error("expected topic node with 'inventory-updates' in qualified name")
	}

	foundSubscribes := false
	for _, edge := range result.Edges {
		if edge.EdgeType == "subscribes" {
			foundSubscribes = true
		}
	}
	if !foundSubscribes {
		t.Error("expected a 'subscribes' edge")
	}
}

func TestEventExtractor_JavaKafkaListener(t *testing.T) {
	e := NewEventExtractor()
	src := `package com.example;

import org.springframework.kafka.annotation.KafkaListener;
import org.springframework.stereotype.Service;

@Service
public class OrderConsumer {
    @KafkaListener(topics = "order-events")
    public void consume(String message) {
        System.out.println("Received: " + message);
    }
}
`
	opts := newTestOpts("src/main/java/com/example/OrderConsumer.java", src)
	result, err := e.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	var topicNodes []types.Node
	for _, n := range result.Nodes {
		if n.Kind == "topic" {
			topicNodes = append(topicNodes, n)
		}
	}
	if len(topicNodes) == 0 {
		t.Fatal("expected at least one topic node, got 0")
	}

	foundTopic := false
	for _, n := range topicNodes {
		if contains(n.QualifiedName, "order-events") {
			foundTopic = true
		}
	}
	if !foundTopic {
		t.Errorf("expected topic node with 'order-events', got: %v", qualifiedNames(topicNodes))
	}

	foundSubscribes := false
	for _, edge := range result.Edges {
		if edge.EdgeType == "subscribes" {
			foundSubscribes = true
		}
	}
	if !foundSubscribes {
		t.Error("expected a 'subscribes' edge")
	}
}

func TestEventExtractor_RabbitMQPython(t *testing.T) {
	e := NewEventExtractor()
	src := `import pika

def setup_rabbitmq():
    connection = pika.BlockingConnection(pika.ConnectionParameters('localhost'))
    channel = connection.channel()

    channel.basic_publish(exchange='', routing_key='task_queue')
    channel.basic_consume(queue='task_queue', on_message_callback=callback)
`
	opts := newTestOpts("workers/rabbitmq.py", src)
	result, err := e.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	var topicNodes []types.Node
	for _, n := range result.Nodes {
		if n.Kind == "topic" {
			topicNodes = append(topicNodes, n)
		}
	}
	if len(topicNodes) == 0 {
		t.Fatal("expected topic nodes for RabbitMQ patterns, got 0")
	}

	foundTopic := false
	for _, n := range topicNodes {
		if contains(n.QualifiedName, "task_queue") {
			foundTopic = true
		}
	}
	if !foundTopic {
		t.Error("expected topic node with 'task_queue' in qualified name")
	}

	// Should have both publishes and subscribes edges
	var edgeTypes []string
	for _, edge := range result.Edges {
		edgeTypes = append(edgeTypes, edge.EdgeType)
	}

	hasPublishes := false
	hasSubscribes := false
	for _, et := range edgeTypes {
		if et == "publishes" {
			hasPublishes = true
		}
		if et == "subscribes" {
			hasSubscribes = true
		}
	}
	if !hasPublishes {
		t.Errorf("expected 'publishes' edge, got edge types: %v", edgeTypes)
	}
	if !hasSubscribes {
		t.Errorf("expected 'subscribes' edge, got edge types: %v", edgeTypes)
	}
}

func TestEventExtractor_GoNatsPublish(t *testing.T) {
	e := NewEventExtractor()
	src := `package main

import "github.com/nats-io/nats.go"

func sendNotification(nc *nats.Conn) {
	nc.Publish("notifications", []byte("hello"))
}
`
	opts := newTestOpts("cmd/notifier/main.go", src)
	result, err := e.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	foundPublishes := false
	for _, edge := range result.Edges {
		if edge.EdgeType == "publishes" {
			foundPublishes = true
		}
	}
	if !foundPublishes {
		t.Error("expected a 'publishes' edge for nats.Publish")
	}

	foundTopic := false
	for _, n := range result.Nodes {
		if n.Kind == "topic" && contains(n.QualifiedName, "notifications") {
			foundTopic = true
		}
	}
	if !foundTopic {
		t.Error("expected topic node with 'notifications'")
	}
}

func TestEventExtractor_JavaKafkaTemplate(t *testing.T) {
	e := NewEventExtractor()
	src := `package com.example;

import org.springframework.kafka.core.KafkaTemplate;

public class OrderProducer {
    private KafkaTemplate<String, String> kafkaTemplate;

    public void sendOrder(String order) {
        kafkaTemplate.send("order-topic", order);
    }
}
`
	opts := newTestOpts("src/main/java/com/example/OrderProducer.java", src)
	result, err := e.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	foundPublishes := false
	for _, edge := range result.Edges {
		if edge.EdgeType == "publishes" {
			foundPublishes = true
		}
	}
	if !foundPublishes {
		t.Error("expected a 'publishes' edge for kafkaTemplate.send")
	}

	foundTopic := false
	for _, n := range result.Nodes {
		if n.Kind == "topic" && contains(n.QualifiedName, "order-topic") {
			foundTopic = true
		}
	}
	if !foundTopic {
		t.Error("expected topic node with 'order-topic'")
	}
}

func TestEventExtractor_EdgeProvenance(t *testing.T) {
	e := NewEventExtractor()
	src := `package main

import "github.com/nats-io/nats.go"

func listen(nc *nats.Conn) {
	nc.Subscribe("events", func(msg *nats.Msg) {})
}
`
	opts := newTestOpts("main.go", src)
	result, err := e.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	for _, edge := range result.Edges {
		if edge.Provenance != "ast_inferred" {
			t.Errorf("expected provenance 'ast_inferred', got %q", edge.Provenance)
		}
		if edge.Confidence != 0.7 {
			t.Errorf("expected confidence 0.7, got %v", edge.Confidence)
		}
	}
}

func TestEventExtractor_EmptyFile(t *testing.T) {
	e := NewEventExtractor()
	opts := newTestOpts("main.go", "package main\n")
	result, err := e.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}
	if len(result.Nodes) != 0 {
		t.Errorf("expected 0 nodes for empty file, got %d", len(result.Nodes))
	}
	if len(result.Edges) != 0 {
		t.Errorf("expected 0 edges for empty file, got %d", len(result.Edges))
	}
}

// --- Helpers ---

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func qualifiedNames(nodes []types.Node) []string {
	var names []string
	for _, n := range nodes {
		names = append(names, n.QualifiedName)
	}
	return names
}
