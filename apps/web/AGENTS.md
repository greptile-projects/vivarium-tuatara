<!-- BEGIN:nextjs-agent-rules -->
# This is NOT the Next.js you know

This version has breaking changes — APIs, conventions, and file structure may all differ from your training data. Read the relevant guide in `node_modules/next/dist/docs/` before writing any code. Heed deprecation notices.
<!-- END:nextjs-agent-rules -->

The Playwright extension journey starts a loopback sample service and enables
the API's explicit `EXTENSION_DEVELOPMENT_ENDPOINTS=1` test boundary. Do not
enable that setting in production; it exists only to exercise live endpoint
ownership challenges without an external test service.
