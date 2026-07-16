# C and cgo

This specification applies to `sdk/c/gizclaw`, the generated C RPC code, the C-facing platform interface, and the cgo bridge connecting Go to C.

## API and ABI

- Maintain ABI compatibility of public headers when breaking change is not explicitly required.
- struct layout, enum value, typedef, callback signature and exported function name all belong to contract.
- Header and source must be synchronized, including declaration, include, ownership, error return and nullability.
- The generated RPC method, message and codec must come from `api/proto/rpc/**/*.proto` and the generated configuration, and the generated results cannot be manually patched.
- platform vtable should clarify required callback, userdata passing and fallback behavior.

## Memory ownership

- Each pointer, buffer and callback parameter must be clearly borrowed, owned or transferred.
- allocation must be paired with free of the same allocator family; if partial initialization fails, the acquired resources must also be released.
- Public API and callback boundaries first check for null before dereference.
- Verify length before pointer arithmetic, allocation, copy, encode and decode, and check the conversion of signed/unsigned, `size_t`, Go length and wire width.
- Buffers can only be saved across callbacks if the contract explicitly guarantees lifetime; stack memory, temporary Go memory, or buffers that have been returned to the caller must not be saved.
- reset/free should be designed to be idempotent when the caller may repeat cleanup.

## cgo bridge

- C must not save ordinary Go pointers for a long time; use `cgo.Handle` or other legal owners when you need to save Go objects across calls.
- Every `cgo.Handle` must be deleted on success, failure and cancellation paths.
- Conversion of C buffers to Go slices handles nil, zero length, maximum length, and validity.
- After the backend, sink, peer or channel is closed, Go can no longer be called back.
- C callback ID, channel label and Go side semantics must be synchronized.

## Testing and Verification

- Pure logic for encode/decode, frame, buffer, key, JSON and signaling using unit test.
- public C API, initialization and platform vtable changes require compile or smoke test.
- Generating code changes requires regeneration check; cgo bridge can use Go test as a reliable verification boundary.
- Override malformed input, boundary length, allocation failure, null pointer and partial cleanup.
- A C surface must not be declared verified simply because an unrelated Go test passes.
