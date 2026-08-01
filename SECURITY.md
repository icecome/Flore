# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 0.1.x   | :white_check_mark: |
| < 0.1   | :x:                |

## Reporting a Vulnerability

We take security seriously. If you discover a security vulnerability, please report it responsibly.

### Reporting Process

1. **Do NOT open a public issue** for security vulnerabilities
2. Email: **werrice@outlook.com**
3. Subject line: **[Security] Flore - <brief description>**
4. Include:
   - Detailed description of the vulnerability
   - Steps to reproduce
   - Potential impact assessment
   - Your contact information for follow-up

### Response Time

- **Acknowledgment**: Within 48 hours
- **Initial assessment**: Within 72 hours
- **Status updates**: Weekly until resolved
- **Resolution target**: Within 30 days for critical issues

### What to Expect

- You will receive an acknowledgment of your report
- We will keep you informed of our progress
- If the vulnerability is accepted, we will work on a fix and credit you (if desired)
- If the vulnerability is declined, we will provide a detailed explanation

## Security Measures

Flore implements the following security controls:

- **SSRF Protection**: DialContext with IP validation, cloud metadata domain blacklist
- **CORS**: Configurable whitelist, default restricted to localhost
- **API Token**: Optional constant-time comparison for API access
- **Input Validation**: Parameterized queries, path sanitization
- **Database**: SQLite with WAL mode, backup with VACUUM INTO

## Disclosure Policy

We follow coordinated disclosure:

1. Fix the vulnerability
2. Release a patched version
3. Publish a security advisory in GitHub Releases
4. Credit the reporter (unless they prefer anonymity)

## Contact

For any security-related questions, please contact: **werrice@outlook.com**
