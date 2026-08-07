import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  async rewrites() {
    const apiOrigin = process.env.API_ORIGIN ?? "http://127.0.0.1:8080";
    return [
      { source: "/api/:path*", destination: `${apiOrigin}/:path*` },
      { source: "/git/:path*", destination: `${apiOrigin}/git/:path*` },
    ];
  },
};

export default nextConfig;
