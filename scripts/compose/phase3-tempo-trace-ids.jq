[
  .traces[]?.traceID,
  .traceID?
]
| .[]
| select(type == "string" and test("^[a-fA-F0-9]+$"))
