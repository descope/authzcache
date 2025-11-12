# Descope AuthZ Cache
A high-performance authorization cache service that accelerates Fine-Grained Authorization (FGA) checks by caching authorization data locally within your cluster.

## Quick Start
Run the service with Docker:
```bash
docker run -d \
  --name authzcache \
  -p 8189:8189 \
  -e HTTP_HOST=0.0.0.0 \
  -e DESCOPE_MANAGEMENT_KEY=your_management_key_here \
  descope/authzcache:latest
```

## Configuration
### Required Environment Variables
- `DESCOPE_MANAGEMENT_KEY` - Your Descope management key for authentication

### Optional Environment Variables
- `DESCOPE_BASE_URL` - Custom Descope base URL (default: production Descope service)
- `CONTAINER_HTTP_PORT` - HTTP gateway port (default: 8189)
- `AUTHZCACHE_SDK_DEBUG_LOG` - Enable debug logging of the internally used Descope SDK (TRUE/FALSE, default: FALSE)
- `AUTHZCACHE_DIRECT_RELATION_CACHE_SIZE_PER_PROJECT` - Direct relation cache size per project (default: 1,000,000)
- `AUTHZCACHE_INDIRECT_RELATION_CACHE_SIZE_PER_PROJECT` - Indirect relation cache size per project (default: 1,000,000)
- `AUTHZCACHE_REMOTE_POLLING_INTERVAL_IN_MILLIS` - Remote polling interval in milliseconds (default: 15,000)

## Ports
- **8189** - HTTP REST API endpoint

## Health Check
The service exposes health check endpoints for container orchestration:
- HTTP: `GET http://localhost:8189/healthz`


## Using From Your Application (Go Example)
To have the Descope SDK use this cache container for accelerating FGA checks, pass its URL via the `FGACacheURL` configuration field when initializing your Descope SDK client. Point the URL to the running container/service inside your local environment or cluster.

For a locally run Docker container (as shown above) the URL will typically be `http://localhost:8189` (or the mapped host/port you selected). In Kubernetes or another orchestrator, use the internal service DNS name, e.g. `http://authzcache.default.svc.cluster.local:8189`.

Example code snippet:
```go
...

  // Replace placeholders with your actual values.
  const (
    YourProjectID                     = "your-project-id"
    YourFGAReadWriteApprovedMGMTKey   = "your-management-key" // must have proper FGA permissions
    URLToThisContainer                = "http://localhost:8189" // or cluster service URL
  )

  // Initialize the Descope SDK client with the AuthZ cache URL.
  err := client.InitDescopeSDKClient(ctx, &azcf.DescopeSDKClientConfig{
    ProjectID:    YourProjectID,
    MgmtKey:      YourFGAReadWriteApprovedMGMTKey,
    FGACacheURL:  URLToThisContainer,
  })
  if err != nil {
    panic(err)
  }

  // Continue with your application logic...
}
```

Notes:
- Ensure the container is reachable from where your code runs (network policy / firewall / service mesh settings).
- The cache automatically syncs with remote authorization data based on the polling interval environment variable.

## License
This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

Alternatively, this project is also available under the Apache License 2.0 - see the [LICENSE-APACHE](LICENSE-APACHE) file for details.

You may choose either license for your use of this software.

## Support
For technical support and questions, please contact your Descope representative.
