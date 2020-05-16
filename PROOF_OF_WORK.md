# Proof of Work Protocol

This document describes the proof of work-based protocol that is used to rate
limit requests (currently, the only endpoint protected by this protocol is
`/report`). The design of the protocol is described
[here](https://www.notion.so/covidwatch/Proof-of-Work-Design-1a17cfed3ff74092996c5c4373be71c6).

## Preliminaries

The core of the protocol is Argon2id, a GPU-resistant key derivation function
(KDF) which is a member of the [Argon2](https://en.wikipedia.org/wiki/Argon2)
family of KDFs.

Argon2 has the following parameters:
- password
- salt
- parallelism
- tagLength
- memorySizeKB
- iterations
- version
- key
- associatedData
- hashType

We define the function, `f`, using the following pseudocode. `challenge` and
`solution` are both 16-byte arrays, and `f` returns an 8-byte array.

```python
def f(challenge, solution):
    return Argon2(
        password       = solution,
        salt           = challenge,
        parallelism    = 1,
        tagLength      = 8,
        memorySizeKB   = 1024,
        iterations     = 1,
        version        = 0x13, # Current version
        key            = [],   # Empty byte array
        associatedData = [],   # Empty byte array
        hashType       = 2,    # Constant indicating Argon2id
    )
```

We define the function, `valid`, using the following pseudocode. `work_factor`
is an unsigned integer, and `key` is an 8-byte array (the output of `f`).
`from_big_endian` is a hypothetical function which interprets its input as an
unsigned integer in big endian byte order. It is equivalent to, e.g., the Go
function
[`encoding/binary.BigEndian.Uint64`](https://golang.org/pkg/encoding/binary/#ByteOrder).

```python
def valid(work_factor, key):
    return from_big_endian(key) % work_factor == 0
```

## Protocol

The client sends a GET request to `/challenge`, and receives a response like
this:

```json
{
   "work_factor" : 1024,
   "nonce" : "54be07e7445880272d5f36cc56c78b6b"
}
```

The client parses `work_factor` as an unsigned integer, and parses `nonce` as a
hexadecimal-encoded array of 16 bytes.

The client solves the challenge by repeatedly producing candidate values for
`solution` until `valid(work_factor, f(nonce, solution))` is true. The mechanism
by which candidate `solution`s are generated is unspecified. So long as each
subsequent `solution` is distinct from all previous ones, the number of
candidates that will need to be generated and checked on average is the same.
For simplicity, it is recommended to initialize `solution` to 0 and increment it
until a solution is found.

Once a solution is found, it is encoded in a string as hexadecimal, and wrapped
in a JSON object like this:

```json
{
   "nonce" : "6e38798e1cf0c5a26fedb35da176a589"
}
```

When a rate-limited request is made to the server, the challenge and solution
are encapsulated together in a single JSON object like this:

```json
{
   "solution" : {
      "nonce" : "6e38798e1cf0c5a26fedb35da176a589"
   },
   "challenge" : {
      "work_factor" : 1024,
      "nonce" : "54be07e7445880272d5f36cc56c78b6b"
   }
}
```

This will usually be sent as a field in a larger JSON object representing the
rate-limited request like this:

```json
{
   "report" : {
      "data" : "9USO+Z30bvZWIKPwZmee0TvkGXBQi7+DqAjtdYZ="
   },
   "challenge" : {
      "solution" : {
         "nonce" : "6e38798e1cf0c5a26fedb35da176a589"
      },
      "challenge" : {
         "nonce" : "54be07e7445880272d5f36cc56c78b6b",
         "work_factor" : 1024
      }
   }
}
```