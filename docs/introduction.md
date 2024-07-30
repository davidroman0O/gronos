# Introduction to Gronos

Gronos is a minimalistic concurrent application management tool for Go applications. It provides a robust framework for managing multiple concurrent applications with features like dynamic application addition, customizable execution modes, and built-in error handling.

## Key Features

- Concurrent application management
- Dynamic application addition and removal
- Multiple execution modes (NonBlocking, ManagedTimeline, BestEffort)
- Built-in error handling and propagation
- Customizable clock system for timed executions
- Generic implementation allowing for type-safe keys
- Iterator for managing sequences of tasks
- Loopable Iterator for advanced task management
- Comprehensive message handling system

## When to Use Gronos

Gronos is particularly useful in scenarios where you need to:

1. Manage multiple concurrent processes or services within a single Go application.
2. Implement complex workflows with interdependent tasks.
3. Create applications with dynamic runtime behavior, where components can be added or removed during execution.
4. Build systems that require fine-grained control over execution modes and error handling.

## Next Steps

- To start using Gronos in your project, check out the [Getting Started](getting-started.md) guide.
- To understand the fundamental concepts of Gronos, read the [Core Concepts](core-concepts.md) section.
- For practical examples of Gronos in action, see the [Examples](examples.md) page.

