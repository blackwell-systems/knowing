package eventextractor

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/java"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/typescript/typescript"

	"github.com/blackwell-systems/knowing/internal/types"
)

const (
	provenance = "ast_inferred"
	confidence = 0.7
)

// supportedExtensions lists file extensions this extractor handles.
var supportedExtensions = map[string]bool{
	".go":   true,
	".ts":   true,
	".tsx":  true,
	".js":   true,
	".jsx":  true,
	".py":   true,
	".java": true,
}

// EventExtractor detects message queue producer and consumer patterns in source
// code using tree-sitter AST parsing.
// Thread-safe: each Extract call creates its own parser.
type EventExtractor struct{}

// NewEventExtractor creates a new EventExtractor.
func NewEventExtractor() *EventExtractor {
	return &EventExtractor{}
}

// parserForExt creates a fresh tree-sitter parser for the given file extension.
func parserForExt(ext string) *sitter.Parser {
	parser := sitter.NewParser()
	switch ext {
	case ".go":
		parser.SetLanguage(golang.GetLanguage())
	case ".ts", ".tsx", ".js", ".jsx":
		parser.SetLanguage(typescript.GetLanguage())
	case ".py":
		parser.SetLanguage(python.GetLanguage())
	case ".java":
		parser.SetLanguage(java.GetLanguage())
	default:
		return nil
	}
	return parser
}

// Name returns the extractor identifier.
func (e *EventExtractor) Name() string {
	return "event-mq"
}

// CanHandle returns true for Go, TypeScript, JavaScript, Python, and Java files
// that are not under node_modules/ directories.
func (e *EventExtractor) CanHandle(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if !supportedExtensions[ext] {
		return false
	}
	// Exclude node_modules
	parts := strings.Split(filepath.ToSlash(path), "/")
	for _, p := range parts {
		if p == "node_modules" {
			return false
		}
	}
	return true
}

// Extract parses the file and detects message queue patterns.
// If opts.ParsedTree is set (shared from a prior extractor), reuses it
// instead of parsing again.
func (e *EventExtractor) Extract(ctx context.Context, opts types.ExtractOptions) (*types.ExtractResult, error) {
	var root *sitter.Node

	// Reuse shared tree if available.
	if opts.ParsedTree != nil {
		if n, ok := opts.ParsedTree.(*sitter.Node); ok {
			root = n
		}
	}

	// Parse ourselves if no shared tree.
	if root == nil {
		ext := strings.ToLower(filepath.Ext(opts.FilePath))
		parser := parserForExt(ext)
		if parser == nil {
			return &types.ExtractResult{}, nil
		}
		tree, err := parser.ParseCtx(ctx, nil, opts.Content)
		if err != nil {
			return &types.ExtractResult{}, nil
		}
		defer tree.Close()
		root = tree.RootNode()
	}

	ext := strings.ToLower(filepath.Ext(opts.FilePath))
	result := &types.ExtractResult{}

	switch ext {
	case ".go":
		e.extractGoPatterns(root, opts, result)
	case ".ts", ".tsx", ".js", ".jsx":
		e.extractTypeScriptPatterns(root, opts, result)
	case ".py":
		e.extractPythonPatterns(root, opts, result)
	case ".java":
		e.extractJavaPatterns(root, opts, result)
	}

	return result, nil
}

// extractGoPatterns detects Kafka (sarama), NATS, SQS, and AMQP patterns in Go code.
func (e *EventExtractor) extractGoPatterns(node *sitter.Node, opts types.ExtractOptions, result *types.ExtractResult) {
	e.walkNode(node, opts, result, func(n *sitter.Node, opts types.ExtractOptions, result *types.ExtractResult) {
		if n.Type() != "call_expression" {
			return
		}
		fnNode := n.ChildByFieldName("function")
		if fnNode == nil {
			return
		}

		fnText := fnNode.Content(opts.Content)

		// NATS patterns: nats.Subscribe, nc.Subscribe, nats.Publish, nc.Publish
		if strings.HasSuffix(fnText, ".Subscribe") || strings.HasSuffix(fnText, ".Publish") {
			args := n.ChildByFieldName("arguments")
			if args == nil {
				return
			}
			topic := e.extractFirstStringArg(args, opts.Content)
			if topic == "" {
				return
			}
			isProducer := strings.HasSuffix(fnText, ".Publish")
			enclosingFunc := e.findEnclosingGoFunc(n, opts)
			e.addTopicEdge(opts, result, topic, enclosingFunc, int(n.StartPoint().Row)+1, isProducer)
			return
		}

		// AMQP patterns: ch.Publish, ch.Consume
		if strings.HasSuffix(fnText, ".Publish") {
			// Already handled above for NATS; this is for AMQP
			// AMQP Publish takes exchange as first arg, routing key as second
			args := n.ChildByFieldName("arguments")
			if args == nil {
				return
			}
			topic := e.extractFirstStringArg(args, opts.Content)
			if topic == "" {
				// Try second arg (routing key)
				topic = e.extractNthStringArg(args, opts.Content, 1)
			}
			if topic == "" {
				return
			}
			enclosingFunc := e.findEnclosingGoFunc(n, opts)
			e.addTopicEdge(opts, result, topic, enclosingFunc, int(n.StartPoint().Row)+1, true)
			return
		}

		if strings.HasSuffix(fnText, ".Consume") {
			args := n.ChildByFieldName("arguments")
			if args == nil {
				return
			}
			topic := e.extractFirstStringArg(args, opts.Content)
			if topic == "" {
				return
			}
			enclosingFunc := e.findEnclosingGoFunc(n, opts)
			e.addTopicEdge(opts, result, topic, enclosingFunc, int(n.StartPoint().Row)+1, false)
			return
		}

		// sarama patterns
		if fnText == "sarama.NewSyncProducer" || fnText == "sarama.NewAsyncProducer" {
			// Producer context but no specific topic yet
			return
		}

		// SQS patterns
		if strings.HasSuffix(fnText, ".SendMessage") {
			// Check if this is an SQS call by looking for QueueUrl in args
			args := n.ChildByFieldName("arguments")
			if args == nil {
				return
			}
			topic := e.extractCompositeFieldStringGo(args, opts.Content, "QueueUrl")
			if topic == "" {
				return
			}
			enclosingFunc := e.findEnclosingGoFunc(n, opts)
			e.addTopicEdge(opts, result, topic, enclosingFunc, int(n.StartPoint().Row)+1, true)
			return
		}

		if strings.HasSuffix(fnText, ".ReceiveMessage") {
			args := n.ChildByFieldName("arguments")
			if args == nil {
				return
			}
			topic := e.extractCompositeFieldStringGo(args, opts.Content, "QueueUrl")
			if topic == "" {
				return
			}
			enclosingFunc := e.findEnclosingGoFunc(n, opts)
			e.addTopicEdge(opts, result, topic, enclosingFunc, int(n.StartPoint().Row)+1, false)
			return
		}
	})

	// Also check for sarama ProducerMessage composite literals with Topic field
	e.walkNode(node, opts, result, func(n *sitter.Node, opts types.ExtractOptions, result *types.ExtractResult) {
		if n.Type() != "composite_literal" {
			return
		}
		typeNode := n.ChildByFieldName("type")
		if typeNode == nil {
			return
		}
		typeText := typeNode.Content(opts.Content)
		if !strings.Contains(typeText, "ProducerMessage") {
			return
		}
		// Look for Topic field in the literal body
		body := n.ChildByFieldName("body")
		if body == nil {
			return
		}
		topic := e.extractLiteralFieldString(body, opts.Content, "Topic")
		if topic == "" {
			return
		}
		enclosingFunc := e.findEnclosingGoFunc(n, opts)
		e.addTopicEdge(opts, result, topic, enclosingFunc, int(n.StartPoint().Row)+1, true)
	})
}

// extractTypeScriptPatterns detects kafkajs and NestJS decorator patterns.
func (e *EventExtractor) extractTypeScriptPatterns(node *sitter.Node, opts types.ExtractOptions, result *types.ExtractResult) {
	e.walkNode(node, opts, result, func(n *sitter.Node, opts types.ExtractOptions, result *types.ExtractResult) {
		switch n.Type() {
		case "call_expression":
			fnNode := n.ChildByFieldName("function")
			if fnNode == nil {
				return
			}
			fnText := fnNode.Content(opts.Content)

			// producer.send({ topic: "..." })
			if strings.HasSuffix(fnText, ".send") {
				args := n.ChildByFieldName("arguments")
				if args == nil {
					return
				}
				topic := e.extractObjectFieldStringTS(args, opts.Content, "topic")
				if topic == "" {
					return
				}
				enclosingFunc := e.findEnclosingTSFunc(n, opts)
				e.addTopicEdge(opts, result, topic, enclosingFunc, int(n.StartPoint().Row)+1, true)
				return
			}

			// consumer.subscribe({ topic: "..." })
			if strings.HasSuffix(fnText, ".subscribe") {
				args := n.ChildByFieldName("arguments")
				if args == nil {
					return
				}
				topic := e.extractObjectFieldStringTS(args, opts.Content, "topic")
				if topic == "" {
					return
				}
				enclosingFunc := e.findEnclosingTSFunc(n, opts)
				e.addTopicEdge(opts, result, topic, enclosingFunc, int(n.StartPoint().Row)+1, false)
				return
			}

		case "decorator":
			// @MessagePattern("...") or @EventPattern("...")
			// Decorator structure: decorator -> call_expression -> identifier + arguments
			for i := 0; i < int(n.ChildCount()); i++ {
				child := n.Child(i)
				if child.Type() == "call_expression" {
					fnNode := child.ChildByFieldName("function")
					if fnNode == nil {
						continue
					}
					name := fnNode.Content(opts.Content)
					if name == "MessagePattern" || name == "EventPattern" {
						args := child.ChildByFieldName("arguments")
						if args == nil {
							continue
						}
						topic := e.extractFirstStringArg(args, opts.Content)
						if topic == "" {
							continue
						}
						enclosingFunc := e.findEnclosingTSFunc(n, opts)
						e.addTopicEdge(opts, result, topic, enclosingFunc, int(n.StartPoint().Row)+1, false)
					}
				}
			}
		}
	})
}

// extractPythonPatterns detects kafka-python and pika (RabbitMQ) patterns.
func (e *EventExtractor) extractPythonPatterns(node *sitter.Node, opts types.ExtractOptions, result *types.ExtractResult) {
	e.walkNode(node, opts, result, func(n *sitter.Node, opts types.ExtractOptions, result *types.ExtractResult) {
		if n.Type() != "call" {
			return
		}
		fnNode := n.ChildByFieldName("function")
		if fnNode == nil {
			return
		}
		fnText := fnNode.Content(opts.Content)

		args := n.ChildByFieldName("arguments")
		if args == nil {
			return
		}

		switch {
		// KafkaConsumer("topic", ...)
		case fnText == "KafkaConsumer":
			topic := e.extractFirstStringArg(args, opts.Content)
			if topic == "" {
				return
			}
			enclosingFunc := e.findEnclosingPyFunc(n, opts)
			e.addTopicEdge(opts, result, topic, enclosingFunc, int(n.StartPoint().Row)+1, false)

		// producer.send("topic", ...)
		case strings.HasSuffix(fnText, ".send"):
			topic := e.extractFirstStringArg(args, opts.Content)
			if topic == "" {
				return
			}
			enclosingFunc := e.findEnclosingPyFunc(n, opts)
			e.addTopicEdge(opts, result, topic, enclosingFunc, int(n.StartPoint().Row)+1, true)

		// channel.basic_publish(exchange='', routing_key='topic')
		case strings.HasSuffix(fnText, ".basic_publish"):
			topic := e.extractKeywordArg(args, opts.Content, "routing_key")
			if topic == "" {
				return
			}
			enclosingFunc := e.findEnclosingPyFunc(n, opts)
			e.addTopicEdge(opts, result, topic, enclosingFunc, int(n.StartPoint().Row)+1, true)

		// channel.basic_consume(queue='topic')
		case strings.HasSuffix(fnText, ".basic_consume"):
			topic := e.extractKeywordArg(args, opts.Content, "queue")
			if topic == "" {
				return
			}
			enclosingFunc := e.findEnclosingPyFunc(n, opts)
			e.addTopicEdge(opts, result, topic, enclosingFunc, int(n.StartPoint().Row)+1, false)
		}
	})
}

// extractJavaPatterns detects Spring Kafka and JMS patterns.
func (e *EventExtractor) extractJavaPatterns(node *sitter.Node, opts types.ExtractOptions, result *types.ExtractResult) {
	e.walkNode(node, opts, result, func(n *sitter.Node, opts types.ExtractOptions, result *types.ExtractResult) {
		switch n.Type() {
		case "method_invocation":
			// kafkaTemplate.send("topic", ...) or jmsTemplate.send("destination", ...)
			nameNode := n.ChildByFieldName("name")
			if nameNode == nil {
				return
			}
			name := nameNode.Content(opts.Content)
			if name != "send" {
				return
			}
			args := n.ChildByFieldName("arguments")
			if args == nil {
				return
			}
			topic := e.extractFirstStringArg(args, opts.Content)
			if topic == "" {
				return
			}
			enclosingFunc := e.findEnclosingJavaMethod(n, opts)
			e.addTopicEdge(opts, result, topic, enclosingFunc, int(n.StartPoint().Row)+1, true)

		case "annotation":
			// @KafkaListener(topics = "...") or @JmsListener(destination = "...")
			nameNode := n.ChildByFieldName("name")
			if nameNode == nil {
				return
			}
			name := nameNode.Content(opts.Content)

			switch name {
			case "KafkaListener":
				topics := e.extractAnnotationFieldStrings(n, opts.Content, "topics")
				enclosingFunc := e.findEnclosingJavaMethod(n, opts)
				for _, topic := range topics {
					e.addTopicEdge(opts, result, topic, enclosingFunc, int(n.StartPoint().Row)+1, false)
				}

			case "JmsListener":
				topics := e.extractAnnotationFieldStrings(n, opts.Content, "destination")
				enclosingFunc := e.findEnclosingJavaMethod(n, opts)
				for _, topic := range topics {
					e.addTopicEdge(opts, result, topic, enclosingFunc, int(n.StartPoint().Row)+1, false)
				}
			}
		}
	})
}

// walkNode recursively walks the AST and calls the visitor for each node.
func (e *EventExtractor) walkNode(node *sitter.Node, opts types.ExtractOptions, result *types.ExtractResult, visitor func(*sitter.Node, types.ExtractOptions, *types.ExtractResult)) {
	visitor(node, opts, result)
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child != nil {
			e.walkNode(child, opts, result, visitor)
		}
	}
}

// addTopicEdge creates the topic node and edge between the enclosing function and the topic.
func (e *EventExtractor) addTopicEdge(opts types.ExtractOptions, result *types.ExtractResult, topic string, enclosingFunc string, line int, isProducer bool) {
	// Create topic node
	topicQN := buildQualifiedName(opts.RepoURL, opts.FilePath, topic)
	topicHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, topic, "topic")

	topicNode := types.Node{
		NodeHash:      topicHash,
		FileHash:      opts.FileHash,
		QualifiedName: topicQN,
		Kind:          "topic",
		Line:          line,
	}
	result.Nodes = append(result.Nodes, topicNode)

	// Create function node for the enclosing context
	funcHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, enclosingFunc, "function")
	funcQN := fmt.Sprintf("%s://%s.function.%s", opts.RepoURL, opts.FilePath, enclosingFunc)

	funcNode := types.Node{
		NodeHash:      funcHash,
		FileHash:      opts.FileHash,
		QualifiedName: funcQN,
		Kind:          "function",
		Line:          line,
	}
	result.Nodes = append(result.Nodes, funcNode)

	// Create edge
	var edge types.Edge
	if isProducer {
		// function -> topic (publishes)
		edge = makeEdge(funcHash, topicHash, "publishes")
	} else {
		// topic -> function (subscribes)
		edge = makeEdge(topicHash, funcHash, "subscribes")
	}
	result.Edges = append(result.Edges, edge)
}

// buildQualifiedName constructs the qualified name for a topic node.
// Format: {repoURL}://{filePath}.topic.{topicName}
func buildQualifiedName(repoURL, filePath, topicName string) string {
	return fmt.Sprintf("%s://%s.topic.%s", repoURL, filePath, topicName)
}

// makeEdge creates an edge with the standard provenance and confidence.
func makeEdge(sourceHash, targetHash types.Hash, edgeType string) types.Edge {
	return types.Edge{
		EdgeHash:   types.ComputeEdgeHash(sourceHash, targetHash, edgeType, provenance),
		SourceHash: sourceHash,
		TargetHash: targetHash,
		EdgeType:   edgeType,
		Confidence: confidence,
		Provenance: provenance,
	}
}

// --- Helper functions for extracting string arguments ---

// extractFirstStringArg extracts the first string literal from an argument list node.
func (e *EventExtractor) extractFirstStringArg(args *sitter.Node, content []byte) string {
	return e.extractNthStringArg(args, content, 0)
}

// extractNthStringArg extracts the Nth string literal from an argument list node.
func (e *EventExtractor) extractNthStringArg(args *sitter.Node, content []byte, n int) string {
	count := 0
	for i := 0; i < int(args.ChildCount()); i++ {
		child := args.Child(i)
		if child == nil {
			continue
		}
		str := e.nodeToString(child, content)
		if str != "" {
			if count == n {
				return str
			}
			count++
		}
	}
	return ""
}

// nodeToString extracts a string value from a node if it's a string literal.
func (e *EventExtractor) nodeToString(node *sitter.Node, content []byte) string {
	switch node.Type() {
	case "interpreted_string_literal", "raw_string_literal":
		// Go strings: "value" or `value`
		s := node.Content(content)
		return unquoteString(s)
	case "string", "string_literal", "template_string":
		// TypeScript/JavaScript/Java strings
		s := node.Content(content)
		return unquoteString(s)
	case "string_fragment":
		return node.Content(content)
	}
	// Check children for string content (handles wrapper nodes)
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "string_fragment", "string_content":
			return child.Content(content)
		case "interpreted_string_literal", "raw_string_literal", "string", "string_literal":
			s := child.Content(content)
			return unquoteString(s)
		}
	}
	return ""
}

// extractCompositeFieldStringGo extracts a string value from a Go composite literal field.
// Handles the keyed_element -> literal_element(identifier) : literal_element(string) structure.
func (e *EventExtractor) extractCompositeFieldStringGo(args *sitter.Node, content []byte, fieldName string) string {
	var found string
	e.walkNode(args, types.ExtractOptions{Content: content}, nil, func(n *sitter.Node, _ types.ExtractOptions, _ *types.ExtractResult) {
		if found != "" {
			return
		}
		if n.Type() != "keyed_element" {
			return
		}
		// keyed_element has: literal_element(key) ":" literal_element(value)
		// First child (literal_element) contains an identifier with the key name
		if n.ChildCount() < 3 {
			return
		}
		keyElem := n.Child(0)
		if keyElem == nil {
			return
		}
		// The key is an identifier inside a literal_element
		keyText := ""
		if keyElem.Type() == "literal_element" && keyElem.ChildCount() > 0 {
			keyChild := keyElem.Child(0)
			if keyChild != nil {
				keyText = keyChild.Content(content)
			}
		} else {
			keyText = keyElem.Content(content)
		}
		if keyText != fieldName {
			return
		}
		// Value is the last child (literal_element containing the string)
		valElem := n.Child(int(n.ChildCount()) - 1)
		if valElem == nil {
			return
		}
		s := e.nodeToString(valElem, content)
		if s != "" {
			found = s
		}
	})
	return found
}

// extractLiteralFieldString extracts a string value from a Go composite literal body field.
func (e *EventExtractor) extractLiteralFieldString(body *sitter.Node, content []byte, fieldName string) string {
	return e.extractCompositeFieldStringGo(body, content, fieldName)
}

// extractObjectFieldStringTS extracts a field value from a TypeScript/JS object literal in arguments.
func (e *EventExtractor) extractObjectFieldStringTS(args *sitter.Node, content []byte, fieldName string) string {
	var found string
	e.walkNode(args, types.ExtractOptions{Content: content}, nil, func(n *sitter.Node, _ types.ExtractOptions, _ *types.ExtractResult) {
		if found != "" {
			return
		}
		// Look for property assignments like `topic: "value"`
		if n.Type() == "pair" || n.Type() == "property_assignment" {
			keyNode := n.ChildByFieldName("key")
			if keyNode == nil && n.ChildCount() > 0 {
				keyNode = n.Child(0)
			}
			if keyNode == nil {
				return
			}
			key := keyNode.Content(content)
			if key == fieldName {
				valNode := n.ChildByFieldName("value")
				if valNode == nil && n.ChildCount() > 2 {
					valNode = n.Child(2)
				}
				if valNode != nil {
					s := e.nodeToString(valNode, content)
					if s != "" {
						found = s
					}
				}
			}
		}
	})
	return found
}

// extractKeywordArg extracts a keyword argument value from a Python call's argument list.
func (e *EventExtractor) extractKeywordArg(args *sitter.Node, content []byte, kwName string) string {
	for i := 0; i < int(args.ChildCount()); i++ {
		child := args.Child(i)
		if child == nil {
			continue
		}
		if child.Type() == "keyword_argument" {
			nameNode := child.ChildByFieldName("name")
			if nameNode == nil {
				continue
			}
			if nameNode.Content(content) == kwName {
				valNode := child.ChildByFieldName("value")
				if valNode == nil {
					continue
				}
				s := e.nodeToString(valNode, content)
				if s != "" {
					return s
				}
			}
		}
	}
	return ""
}

// extractAnnotationFieldStrings extracts string values from a Java annotation field.
// Handles both single values: topics = "value"
// and array values: topics = {"value1", "value2"}
func (e *EventExtractor) extractAnnotationFieldStrings(annotation *sitter.Node, content []byte, fieldName string) []string {
	var results []string
	e.walkNode(annotation, types.ExtractOptions{Content: content}, nil, func(n *sitter.Node, _ types.ExtractOptions, _ *types.ExtractResult) {
		if n.Type() == "element_value_pair" {
			keyNode := n.ChildByFieldName("key")
			if keyNode == nil {
				// Try first child as key
				if n.ChildCount() > 0 {
					keyNode = n.Child(0)
				}
			}
			if keyNode == nil {
				return
			}
			if keyNode.Content(content) != fieldName {
				return
			}
			valNode := n.ChildByFieldName("value")
			if valNode == nil && n.ChildCount() > 2 {
				valNode = n.Child(2)
			}
			if valNode == nil {
				return
			}
			// Single string
			s := e.nodeToString(valNode, content)
			if s != "" {
				results = append(results, s)
				return
			}
			// Array initializer: {"val1", "val2"}
			if valNode.Type() == "element_value_array_initializer" || valNode.Type() == "array_initializer" {
				for i := 0; i < int(valNode.ChildCount()); i++ {
					arrChild := valNode.Child(i)
					if arrChild == nil {
						continue
					}
					sv := e.nodeToString(arrChild, content)
					if sv != "" {
						results = append(results, sv)
					}
				}
			}
		}
	})
	return results
}

// --- Enclosing function finders ---

// findEnclosingGoFunc finds the name of the enclosing function declaration.
func (e *EventExtractor) findEnclosingGoFunc(node *sitter.Node, opts types.ExtractOptions) string {
	cur := node.Parent()
	for cur != nil {
		if cur.Type() == "function_declaration" {
			nameNode := cur.ChildByFieldName("name")
			if nameNode != nil {
				return nameNode.Content(opts.Content)
			}
		}
		if cur.Type() == "method_declaration" {
			nameNode := cur.ChildByFieldName("name")
			if nameNode != nil {
				return nameNode.Content(opts.Content)
			}
		}
		cur = cur.Parent()
	}
	return "_file_scope_"
}

// findEnclosingTSFunc finds the name of the enclosing function/method in TypeScript.
func (e *EventExtractor) findEnclosingTSFunc(node *sitter.Node, opts types.ExtractOptions) string {
	cur := node.Parent()
	for cur != nil {
		switch cur.Type() {
		case "function_declaration", "method_definition":
			nameNode := cur.ChildByFieldName("name")
			if nameNode != nil {
				return nameNode.Content(opts.Content)
			}
		case "variable_declarator":
			// Arrow function assigned to variable
			nameNode := cur.ChildByFieldName("name")
			if nameNode != nil {
				return nameNode.Content(opts.Content)
			}
		}
		cur = cur.Parent()
	}
	return "_file_scope_"
}

// findEnclosingPyFunc finds the name of the enclosing function/method in Python.
func (e *EventExtractor) findEnclosingPyFunc(node *sitter.Node, opts types.ExtractOptions) string {
	cur := node.Parent()
	for cur != nil {
		if cur.Type() == "function_definition" {
			nameNode := cur.ChildByFieldName("name")
			if nameNode != nil {
				return nameNode.Content(opts.Content)
			}
		}
		cur = cur.Parent()
	}
	return "_file_scope_"
}

// findEnclosingJavaMethod finds the name of the enclosing method in Java.
func (e *EventExtractor) findEnclosingJavaMethod(node *sitter.Node, opts types.ExtractOptions) string {
	cur := node.Parent()
	for cur != nil {
		if cur.Type() == "method_declaration" {
			nameNode := cur.ChildByFieldName("name")
			if nameNode != nil {
				return nameNode.Content(opts.Content)
			}
		}
		cur = cur.Parent()
	}
	return "_file_scope_"
}

// unquoteString removes surrounding quotes from a string literal.
func unquoteString(s string) string {
	if len(s) < 2 {
		return ""
	}
	if (s[0] == '"' && s[len(s)-1] == '"') ||
		(s[0] == '\'' && s[len(s)-1] == '\'') ||
		(s[0] == '`' && s[len(s)-1] == '`') {
		return s[1 : len(s)-1]
	}
	return ""
}
