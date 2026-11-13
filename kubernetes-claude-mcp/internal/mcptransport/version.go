package mcptransport

// Version is what the server reports to MCP clients. The binary is built
// without version ldflags, so it is written here; TestVersionMatchesChart keeps
// it equal to the Helm chart's appVersion, which the release workflow already
// requires to equal the release tag.
const Version = "1.6.0"
