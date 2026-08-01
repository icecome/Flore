# Contributing to Flore

Thank you for your interest in contributing to Flore! This document provides guidelines for contributing.

## Code of Conduct

- Be respectful and constructive
- Focus on technical merits, not personal attributes
- Accept constructive criticism gracefully
- Show empathy towards other community members

## Getting Started

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/your-feature`)
3. Make your changes
4. Run tests and linting
5. Commit with clear messages
6. Push and open a Pull Request

## Development Setup

```bash
# Install dependencies
npm install

# Run in development mode
npm run dev

# Build for desktop
npm run build:desktop
```

## Coding Standards

### Go (Backend)
- Follow [Effective Go](https://go.dev/doc/effective_go)
- Use `gofmt` for formatting
- Write tests for new functionality
- Use parameterized queries for all SQL

### TypeScript/React (Frontend)
- Use TypeScript strictly (no `any`)
- Follow [React Hooks](https://react.dev/reference/react)
- Use Tailwind CSS for styling
- Write component tests where appropriate

## Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: add new feature
fix: fix bug
docs: update documentation
style: format code
refactor: refactor code
test: add tests
chore: update dependencies
```

## Pull Request Process

1. Update documentation if needed
2. Add tests for new functionality
3. Ensure CI passes
4. Request review from maintainers
5. Squash commits before merge

## Security

- Do not commit sensitive data (keys, tokens, passwords)
- Report security vulnerabilities via [SECURITY.md](SECURITY.md)
- Use parameterized queries for all database operations

## License

By contributing, you agree that your contributions will be licensed under the GNU General Public License v3.0.
