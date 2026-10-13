# Makes a minisign key and signature with Python's own Ed25519 and BLAKE2b,
# an implementation independent of the Go one: python3 gen.py
import base64, hashlib, os
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
from cryptography.hazmat.primitives import serialization

sk = Ed25519PrivateKey.generate()
pk = sk.public_key().public_bytes(serialization.Encoding.Raw, serialization.PublicFormat.Raw)
key_id = os.urandom(8)
msg = b'{"version":"1.2.3","files":{}}\n'
open("message.json", "wb").write(msg)
open("key.pub", "w").write("untrusted comment: minisign public key\n" + base64.b64encode(b"Ed" + key_id + pk).decode() + "\n")
sig = sk.sign(hashlib.blake2b(msg, digest_size=64).digest())
comment = "timestamp:1760000000\tfile:message.json\thashed"
glob = sk.sign(sig + comment.encode())
open("message.json.minisig", "w").write(
    "untrusted comment: signature from minisign secret key\n"
    + base64.b64encode(b"ED" + key_id + sig).decode() + "\n"
    + "trusted comment: " + comment + "\n"
    + base64.b64encode(glob).decode() + "\n")
