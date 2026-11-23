# GEMINI.md

## Project Overview

This project is a WebRTC server written in Go. It uses gRPC for signaling and Redis for managing user sessions. The core WebRTC functionality is implemented using the `pion/webrtc` library. The server facilitates real-time communication between clients by handling the exchange of session descriptions and ICE candidates. The application is structured using the `uber-go/fx` dependency injection framework.

## Building and Running

To build and run the project, you need to have Go installed.

### Dependencies

The project's dependencies are managed using Go modules. You can install them by running:

```sh
go mod tidy
```

### Configuration

The application requires a `.env` file in the root directory with the following environment variables:

```
HEALTH_PORT=your_health_port
SERVER_PORT=your_server_port
NAT_IP=your_nat_ip
```

### Running the server

To run the server, execute the following command from the root directory:

```sh
go run cmd/webrtc/main.go
```

## Development Conventions

### Code Style

The codebase follows standard Go conventions.

### Testing

There are no explicit testing practices evident from the file structure.

### Contribution Guidelines

There are no explicit contribution guidelines.
