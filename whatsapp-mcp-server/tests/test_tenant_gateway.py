import sys
import time

from tenant_gateway import RuntimeRegistry

TEST_IDENTITY = "11111111-1111-4111-8111-111111111111"
TEST_BRIDGE_TOKEN = "bridge-token-for-tests"

# A stand-in for the Go bridge: it says something, then answers /api/status the way
# RuntimeRegistry waits for before it hands the runtime out.
FAKE_BRIDGE = f"""#!{sys.executable}
import os, sys
from http.server import BaseHTTPRequestHandler, HTTPServer

class Status(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.end_headers()
        self.wfile.write(b"{{}}")

    def log_message(self, *args):
        pass

print("qr_timeout: QR codes stopped before the pairing finished", flush=True)
HTTPServer(("127.0.0.1", int(os.environ["WHATSAPP_BRIDGE_PORT"])), Status).serve_forever()
"""


# On Aura's lab VM (2026-10-14) a bridge served one dead QR code for hours and said why to nobody:
# its output went to /dev/null, so the only way to read it was to read the code. What a bridge
# says reaches the container's log, tagged with the tenant it serves.
def test_a_bridge_says_what_it_does_in_the_container_log(tmp_path, capfd):
    binary = tmp_path / "fake-bridge"
    binary.write_text(FAKE_BRIDGE)
    binary.chmod(0o755)
    registry = RuntimeRegistry(tmp_path / "root", binary, TEST_BRIDGE_TOKEN)
    try:
        registry.get(TEST_IDENTITY)
        deadline = time.monotonic() + 5
        err = ""
        while time.monotonic() < deadline and "qr_timeout" not in err:
            time.sleep(0.05)
            err += capfd.readouterr().err
    finally:
        registry.close()

    assert f"whatsapp-bridge[{TEST_IDENTITY[:8]}]: qr_timeout: QR codes stopped" in err
