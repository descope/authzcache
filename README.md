# Descope AuthZ Cache
A high-performance authorization cache service that accelerates Fine-Grained Authorization (FGA) checks by caching authorization data locally within your cluster.
## Quick Start
Run the service with Docker:
```bash
docker run -d \
  --name authzcache \
  -p 8189:8189 \
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
- HTTP: `GET http://localhost:8189/health`
## Support
For technical support and questions, please contact your Descope representative.