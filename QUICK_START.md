# Quick Start Guide - Distributed KV Cache

**Version:** Phase 4 Complete  
**Last Updated:** July 23, 2026

---

## 🚀 Quick Start (30 seconds)

### Single Node (Development)
```bash
go run ./tcp
# Server starts on :9000
```

### 3-Node Cluster (Distributed Mode)
```bash
cd test
start_cluster.bat
# 3 nodes start on :5000, :5001, :5002
```

---

## 📋 Commands

### Client Connection
```bash
# Using netcat
nc localhost 5000

# Using telnet
telnet localhost 5000
```

### Basic Operations
```
PING
→ PONG

SET mykey 5
hello
→ OK

GET mykey
→ hello

SET session 5 EX 60
token
→ OK

DELETE mykey
→ OK

GET mykey
→ NULL
```

---

## ⚙️ Configuration

### Environment Variables
```bash
# Enable distributed mode
set CLUSTER_ROUTING=true

# Set node identity
set CLUSTER_NODE_ID=node1
```

### cluster.json
Located at: `cluster/cluster.json`
```json
{
    "virtual_nodes": 100,
    "nodes": [
        {"id": "node1", "address": "localhost:5000"},
        {"id": "node2", "address": "localhost:5001"},
        {"id": "node3", "address": "localhost:5002"}
    ]
}
```

---

## 🧪 Testing

### Run Unit Tests
```bash
go test ./...
```

### Run Cluster Test
```bash
cd test
go run test_cluster.go
```

### Manual Testing
```bash
# Terminal 1: Start node 1
set CLUSTER_NODE_ID=node1 && set CLUSTER_ROUTING=true && go run ./tcp

# Terminal 2: Start node 2
set CLUSTER_NODE_ID=node2 && set CLUSTER_ROUTING=true && go run ./tcp

# Terminal 3: Start node 3
set CLUSTER_NODE_ID=node3 && set CLUSTER_ROUTING=true && go run ./tcp

# Terminal 4: Connect and test
nc localhost 5000
SET test1 6
value1
GET test1
```

---

## 📊 Monitoring

### Check Server Status
```bash
# Connect and send PING
echo PING | nc localhost 5000
# → PONG means server is running
```

### View Logs
- Server logs to stdout
- Look for "Listening on" to confirm startup
- Forwarding logged as "forward to <address>"

---

## 🐛 Troubleshooting

### Server won't start
```bash
# Check if port is in use
netstat -an | findstr :5000

# Kill process using port (Windows)
netstat -ano | findstr :5000
taskkill /PID <PID> /F
```

### Can't connect to cluster
- ✅ Check CLUSTER_ROUTING=true is set
- ✅ Check CLUSTER_NODE_ID matches cluster.json
- ✅ Verify all nodes are running
- ✅ Check firewall isn't blocking ports

### Requests failing
- Check server logs for errors
- Verify key/value sizes within limits:
  - Max key size: 4KB
  - Max value size: 1MB
- Try PING to verify connection

---

## 📚 Documentation

- **Full documentation:** `AI_PROJECT_REFERENCE.md`
- **Phase 4 details:** `PHASE4_COMPLETION.md`
- **Changes:** `CHANGELOG.md`
- **Project overview:** `README.md`

---

## 🔧 Development

### Build Binary
```bash
go build -o cache.exe ./tcp
```

### Run Binary
```bash
# Single node
cache.exe

# Cluster mode
set CLUSTER_NODE_ID=node1 && set CLUSTER_ROUTING=true && cache.exe
```

### Add New Command
1. Update `parser/parser.go` - Add parsing logic
2. Update `tcp/main.go` - Add execution logic
3. Update `storage/cache.go` - Implement if needed
4. Add to WAL if it mutates state
5. Write tests

---

## 🎯 Common Use Cases

### Session Storage
```
SET session:user123 36 EX 3600
{"user_id":123,"auth":"token_abc"}
```

### Caching API Responses
```
SET api:users:list 150 EX 300
[{"id":1,"name":"Alice"},{"id":2,"name":"Bob"}]
```

### Rate Limiting
```
SET ratelimit:user456:minute 1 EX 60
1
```

---

## 📈 Limits

| Resource | Limit |
|----------|-------|
| Max key size | 4 KB |
| Max value size | 1 MB |
| Max keys | 100 (configurable) |
| Max memory | 512 MB (configurable) |
| Max connections | 128 concurrent |
| Command timeout | 30s read, 10s write |

---

## 🚦 Status Indicators

### Response Messages
- `OK` - Operation successful
- `PONG` - Server alive
- `NULL` - Key not found
- `ERR <message>` - Error occurred
- `ERROR <message>` - Command parse error

### Server States
- `Listening on :5000` - Ready to accept connections
- `Client connected` - New client accepted
- `forward to <address>` - Request forwarded to another node

---

## 💡 Tips

1. **Start simple:** Test single-node mode first
2. **Use PING:** Verify connectivity before operations
3. **Check logs:** Server logs show forwarding and errors
4. **Test forwarding:** Connect to node1, SET keys that hash to different nodes
5. **Monitor memory:** Check Stats() periodically (future feature)

---

## 🔗 Quick Links

- **GitHub Issues:** Track bugs and features
- **Architecture Docs:** See `AI_PROJECT_REFERENCE.md`
- **API Reference:** Command protocol in `README.md`
- **Testing Guide:** See `PHASE4_COMPLETION.md`

---

**Need Help?**
- Check `AI_PROJECT_REFERENCE.md` for detailed architecture
- Review `PHASE4_COMPLETION.md` for distributed mode details
- Run automated tests: `go test ./...`
- Test cluster: `cd test && go run test_cluster.go`

---

**Version:** Phase 4 & 5 Complete  
**Status:** ✅ Production-ready for distributed deployments  
**Next:** Phase 6 (Replication & Reliability)
