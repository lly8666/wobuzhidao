"""Unit test scoped adapter in Actions; historical experiment is unchanged."""
import hashlib,json,tempfile,unittest
from pathlib import Path
from prepare_crypto_packet_harness import adapt,REPLACEMENTS

class TestCryptoAdapter(unittest.TestCase):
 def fixture(self,dir, text):
    p=Path(dir)/"generated.sh"
    p.write_text(text)
    receipt={"schema":"wbd-large-mtu-harness-template/v1","generated_sha256":hashlib.sha256(text.encode()).hexdigest(),"original_sha256":"fixture-original"}
    Path(str(p)+".receipt.json").write_text(json.dumps(receipt))
    return p
 def test_exact_two_selector_rewrites_receipt(self):
    with tempfile.TemporaryDirectory() as tmp:
      p=self.fixture(tmp,"\n".join(x[0] for x in REPLACEMENTS))
      before=hashlib.sha256(p.read_bytes()).hexdigest()
      adapt(p)
      new=p.read_text()
      receipt=json.loads(Path(str(p)+".receipt.json").read_text())
      self.assertEqual(receipt["base_fec_simd_generated_sha256"],before)
      self.assertEqual(receipt["generated_sha256"],hashlib.sha256(p.read_bytes()).hexdigest())
      self.assertEqual(receipt["original_sha256"],"fixture-original")
      for old,updated in REPLACEMENTS:
       self.assertNotIn(old,new)
       self.assertEqual(new.count(updated),1)
 def test_mismatched_receipt_rejected_without_mutation(self):
    with tempfile.TemporaryDirectory() as tmp:
      p=self.fixture(tmp,"\n".join(x[0] for x in REPLACEMENTS))
      receipt=Path(str(p)+".receipt.json")
      obj=json.loads(receipt.read_text());obj["generated_sha256"]="0"*64;receipt.write_text(json.dumps(obj))
      original=p.read_bytes()
      with self.assertRaises(ValueError):adapt(p)
      self.assertEqual(p.read_bytes(),original)
 def test_missing_or_multiple_selector_rejected(self):
    for extra in (0,2):
     with self.subTest(count=extra),tempfile.TemporaryDirectory() as tmp:
      p=self.fixture(tmp,(REPLACEMENTS[0][0]+"\n")*extra+REPLACEMENTS[1][0])
      prior=p.read_bytes()
      with self.assertRaises(ValueError):adapt(p)
      self.assertEqual(p.read_bytes(),prior)
