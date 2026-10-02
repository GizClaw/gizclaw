"""Run the standalone Mem0 service."""

import argparse
import os


def main():
    parser = argparse.ArgumentParser(description="GizClaw Mem0 HTTP service")
    parser.add_argument("--config", help="YAML or JSON configuration file")
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=8000)
    parser.add_argument("--version", action="version", version=os.environ.get("GIZCLAW_MEM0_VERSION", "dev"))
    args = parser.parse_args()
    if args.config:
        os.environ["MEM0_CONFIG"] = args.config
    import uvicorn
    uvicorn.run("gizclaw_mem0.server:app", host=args.host, port=args.port)


if __name__ == "__main__":
    main()
