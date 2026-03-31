import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  output: "export",
  images: {
    unoptimized: true,
  },
  // Disable trailing slashes for cleaner embed.FS serving
  trailingSlash: false,
};

export default nextConfig;
