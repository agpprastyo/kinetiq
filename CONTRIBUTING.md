# Contributing to Kinetiq

Thank you for your interest in contributing to Kinetiq Community Edition!

## Getting Started
1. Fork the [kinetiq](https://github.com/agpprastyo/kinetiq) repository
2. Clone your fork and create a feature branch
3. Follow the [Code of Conduct](CODE_OF_CONDUCT.md)
4. Submit a pull request

## Development Setup
```bash
bun install
cd apps/api && go mod download
```

## Testing
```bash
bun test          # Web/Auth tests
cd apps/api && go test ./...  # API tests
```

## Pull Request Guidelines
- Sign off your commits with `git commit -s` (DCO)
- Follow existing code style
- Add tests for new functionality
- Update documentation as needed
- Keep PRs focused and small

## Issue Guidelines
- Use issue templates
- Search existing issues first
- Provide reproduction steps for bugs
- Include environment details

## License
By contributing, you agree that your contributions will be licensed under the Apache License 2.0.
