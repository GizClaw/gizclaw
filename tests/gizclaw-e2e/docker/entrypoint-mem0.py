"""Start the real Mem0 service using the standard E2E credential file."""

import os
from dotenv import dotenv_values

credentials = dotenv_values(os.environ["MEM0_CREDENTIAL_FILE"])
key = credentials.get("GIZCLAW_E2E_VOLC_ARK_API_KEY", "")
if not key:
    raise SystemExit("missing GIZCLAW_E2E_VOLC_ARK_API_KEY")
os.environ["VOLC_ARK_API_KEY"] = key
os.execvp("python", ["python", "-m", "gizclaw_mem0", "--host", "0.0.0.0", "--port", "8000"])
