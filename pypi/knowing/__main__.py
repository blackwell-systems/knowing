"""Entry point for the knowing CLI wrapper.

Downloads and runs the platform-specific knowing binary from GitHub Releases.
"""

import os
import platform
import subprocess
import sys
import urllib.request
import zipfile
import tarfile
import tempfile
from pathlib import Path


REPO = "blackwell-systems/knowing"
VERSION = "0.1.0"


def get_binary_path() -> Path:
    """Get the path where the knowing binary is cached."""
    cache_dir = Path.home() / ".cache" / "knowing" / VERSION
    cache_dir.mkdir(parents=True, exist_ok=True)

    binary_name = "knowing"
    if platform.system() == "Windows":
        binary_name = "knowing.exe"

    return cache_dir / binary_name


def detect_platform() -> str:
    """Detect OS and architecture for binary download."""
    system = platform.system().lower()
    machine = platform.machine().lower()

    if system == "darwin":
        os_name = "darwin"
    elif system == "linux":
        os_name = "linux"
    elif system == "windows":
        os_name = "windows"
    else:
        raise RuntimeError(f"Unsupported OS: {system}")

    if machine in ("x86_64", "amd64"):
        arch = "amd64"
    elif machine in ("aarch64", "arm64"):
        arch = "arm64"
    else:
        raise RuntimeError(f"Unsupported architecture: {machine}")

    return f"{os_name}_{arch}"


def download_binary(binary_path: Path) -> None:
    """Download the knowing binary from GitHub Releases."""
    plat = detect_platform()
    ext = "zip" if "windows" in plat else "tar.gz"
    url = f"https://github.com/{REPO}/releases/download/v{VERSION}/knowing_{VERSION}_{plat}.{ext}"

    print(f"Downloading knowing v{VERSION} for {plat}...", file=sys.stderr)

    with tempfile.NamedTemporaryFile(suffix=f".{ext}", delete=False) as tmp:
        urllib.request.urlretrieve(url, tmp.name)
        tmp_path = tmp.name

    try:
        if ext == "tar.gz":
            with tarfile.open(tmp_path, "r:gz") as tar:
                for member in tar.getmembers():
                    if member.name.endswith("knowing"):
                        f = tar.extractfile(member)
                        if f:
                            binary_path.write_bytes(f.read())
                            binary_path.chmod(0o755)
                            break
        else:
            with zipfile.ZipFile(tmp_path, "r") as zf:
                for name in zf.namelist():
                    if name.endswith("knowing.exe") or name.endswith("knowing"):
                        binary_path.write_bytes(zf.read(name))
                        binary_path.chmod(0o755)
                        break
    finally:
        os.unlink(tmp_path)

    if not binary_path.exists():
        raise RuntimeError(f"Failed to extract knowing binary from {url}")

    print(f"Installed knowing to {binary_path}", file=sys.stderr)


def main() -> None:
    """Run the knowing binary, downloading if needed."""
    binary_path = get_binary_path()

    if not binary_path.exists():
        download_binary(binary_path)

    result = subprocess.run(
        [str(binary_path)] + sys.argv[1:],
        stdin=sys.stdin,
        stdout=sys.stdout,
        stderr=sys.stderr,
    )
    sys.exit(result.returncode)


if __name__ == "__main__":
    main()
