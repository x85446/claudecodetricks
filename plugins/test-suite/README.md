# test-suite Plugin

Expert test engineer agent for comprehensive test suite design and implementation.

## Overview

The `test-suite` plugin provides an AI-powered test engineering specialist that helps you design, implement, and improve test suites across multiple programming languages and testing frameworks.

## Features

- **Comprehensive Test Strategy**: Analyzes codebases and designs complete test coverage plans
- **Multi-Language Support**: Go, JavaScript/TypeScript, Python, and more
- **Framework Expertise**: Jest, Vitest, pytest, Go testing, React Testing Library, Playwright
- **Best Practices**: Follows industry-standard testing patterns (AAA, table-driven, etc.)
- **Coverage Analysis**: Identifies testing gaps and suggests improvements
- **Test Quality**: Writes clear, maintainable, isolated tests

## Installation

```bash
# Install the plugin
/plugin install test-suite@claudecodetricks
```

## Usage

### Invoke the Agent

The `test-expert` agent can be invoked in several ways:

**Explicit Request:**
```
Use the test-expert agent to write unit tests for my authentication module
```

**Contextual Activation:**
Claude Code will automatically activate the agent when you mention testing tasks:
```
I need to add tests for the user service
Write integration tests for the API endpoints
```

**Direct Agent Call:**
```
/agents test-expert
```

### Example Tasks

1. **Create Test Suite**
   ```
   Create a comprehensive test suite for src/utils/validation.js
   ```

2. **Add Missing Tests**
   ```
   Review src/internal/git/conventional.go and add any missing test cases
   ```

3. **Test Strategy Design**
   ```
   Design a testing strategy for our e-commerce checkout flow
   ```

4. **Coverage Analysis**
   ```
   Analyze test coverage for the authentication module and suggest improvements
   ```

## Agent Capabilities

### Test Types
- Unit tests
- Integration tests
- End-to-end tests
- Table-driven tests
- Property-based tests

### Testing Patterns
- **AAA Pattern**: Arrange, Act, Assert
- **Table-Driven Tests**: For testing multiple scenarios (especially Go)
- **Mocking**: External dependencies, HTTP clients, databases
- **Fixtures**: Test data generation and management

### Language-Specific Features

**Go:**
- Standard `testing` package
- Table-driven test patterns
- Subtests with `t.Run()`
- Race detection with `-race`
- HTTP handler testing with `httptest`

**JavaScript/TypeScript:**
- Jest/Vitest for unit tests
- React Testing Library for components
- Playwright/Cypress for E2E
- MSW for API mocking

**Python:**
- pytest framework
- unittest.mock for mocking
- pytest-cov for coverage
- hypothesis for property-based testing

## Coverage Guidelines

The agent follows industry-standard coverage targets:

- **Critical Code**: 90%+ (authentication, payments, data integrity)
- **Business Logic**: 80%+
- **UI Components**: 60%+
- **Utilities**: 70%+

## Test Quality Standards

The agent ensures tests are:

1. **Isolated**: No dependencies on execution order
2. **Fast**: Unit tests under 100ms when possible
3. **Clear**: Descriptive names and assertion messages
4. **Simple**: No complex logic in tests
5. **Deterministic**: Same input always produces same result
6. **Behavior-Focused**: Tests public APIs, not implementation details

## Example Output

When you ask the agent to create tests, it will:

1. Analyze the source code to understand behavior
2. Identify test scenarios (happy path, edge cases, errors)
3. Write complete, runnable test files
4. Provide coverage assessment
5. Suggest additional testing if needed

## Configuration

No configuration required. The agent automatically:
- Detects your programming language
- Identifies testing frameworks in use
- Adapts to your project's testing conventions
- Follows your existing test file organization

## Agent Metadata

- **Name**: test-expert
- **Model**: Claude Sonnet
- **Tools**: Read, Write, Edit, Grep, Glob, Bash
- **Color**: Green (in Claude Code UI)
- **Category**: Testing

## Tips

1. **Be Specific**: The more context you provide, the better the tests
2. **Review Output**: Always review generated tests for your specific needs
3. **Iterative Improvement**: Ask for refinements or additional scenarios
4. **Coverage First**: Start with critical paths, then expand coverage

## Related Plugins

- **session-hooks**: Includes git-committer for auto-committing test files
- Pairs well with code review and CI/CD workflows

## License

MIT

## Author

x85446 - [GitHub](https://github.com/x85446)
