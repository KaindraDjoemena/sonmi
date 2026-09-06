import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  images: {
    // Sources are short-lived presigned S3 GET URLs from GET /api/journals.
    // Kept tight: https + objects under /daily/ only. Swap the hostname for the
    // exact "<bucket>.s3.<region>.amazonaws.com" if you want it stricter.
    remotePatterns: [
      { protocol: "https", hostname: "**.amazonaws.com", pathname: "/daily/**" },
    ],
  },
};

export default nextConfig;
