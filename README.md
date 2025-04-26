# Solana Balance Reporter

A Go application that fetches Solana token balances for a list of wallet addresses hourly and sends CSV reports via email.

## Features

- 🕒 Hourly balance fetching for configured token
- 📄 CSV report generation with wallet addresses and balances
- 📧 Email reports via Amazon SES with CSV attachments
- 🔄 Dynamic address list loading (no restart needed when adding addresses)
- 🧠 Concurrent balance fetching with configurable limits
- 🔁 Exponential backoff retry mechanism for API and email failures
- 📊 Detailed logging with hourly rotation
- 🐳 Docker containerization support

## Project Structure

```
solana-balance-reporter/
├── cmd/
│   └── main.go                 # App entry point
├── internal/
│   ├── config/                 # Configuration handling
│   ├── csvwriter/              # CSV file creation
│   ├── logger/                 # Logging utilities
│   ├── mailer/                 # Email sending functionality
│   ├── reader/                 # Address file loading
│   └── solana/                 # Solana RPC client
├── logs/                       # Log files directory
├── csv/                        # Generated CSV files directory
├── addresses.txt               # Wallet addresses list
├── .env                        # Environment configuration
├── Dockerfile                  # Container definition
├── docker-compose.yml          # Container orchestration
└── README.md                   # This documentation
```

## Setup and Configuration

### Prerequisites

- Go 1.16+ (for local development)
- Docker and Docker Compose (for containerized deployment)
- Solana RPC endpoint
- SMTP server credentials (Amazon SES or similar)

### Configuration

1. Copy `.env.example` to `.env` and update with your settings:

```
# Solana RPC settings
SOLANA_RPC_URL=https://your-rpc-endpoint
TOKEN_MINT_ADDRESS=your-token-mint-address

# Fetch interval in minutes (60 = 1 hour)
FETCH_INTERVAL_MINUTES=60

# Performance settings
RPC_TIMEOUT_SECONDS=10
MAX_RETRIES=3
CONCURRENCY_LIMIT=20

# Email settings
SMTP_SERVER=email-smtp.us-east-1.amazonaws.com
SMTP_PORT=587
SMTP_USERNAME=your-smtp-username
SMTP_PASSWORD=your-smtp-password
EMAIL_FROM=sender@example.com
EMAIL_TO=recipient1@example.com,recipient2@example.com
```

2. Update `addresses.txt` with the Solana wallet addresses you want to monitor (one per line).

## Deployment

### Using Docker Compose (Recommended)

1. Build and start the container:

```bash
docker-compose up -d
```

2. View logs:

```bash
docker-compose logs -f
```

3. Stop the service:

```bash
docker-compose down
```

### Building and Running Locally

1. Build the application:

```bash
go build -o solana-balance-reporter ./cmd
```

2. Run the application:

```bash
./solana-balance-reporter
```

## Monitoring

- Check the latest log file in the `logs/` directory
- Review generated CSV files in the `csv/` directory
- Email reports are sent hourly to configured recipients

## Adding New Addresses

Simply add new wallet addresses to the `addresses.txt` file. The application reloads the file before each run, so no restart is required.

## License

MIT License 