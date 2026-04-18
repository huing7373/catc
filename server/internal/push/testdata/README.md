# testdata — NON-PRODUCTION

`test_key.p8` is a locally-generated ECDSA P-256 private key used solely
by `apns_sender_test.go` to exercise the `.p8` parse path of
`NewApnsClient`. It is **not** an Apple-issued APNs authentication key and
is unsafe for any production use — Apple's push service will reject it
because no team / key ID in the real APNs tenancy corresponds to it.

Regenerate with:

```
openssl ecparam -name prime256v1 -genkey -noout -out test_key.p8
openssl pkcs8 -topk8 -nocrypt -in test_key.p8 -out test_key.p8.pkcs8
mv test_key.p8.pkcs8 test_key.p8
```

Keep the file committed to the repo — tests reference it by relative path
(`testdata/test_key.p8`) via `t.TempDir()` copy or direct open.
