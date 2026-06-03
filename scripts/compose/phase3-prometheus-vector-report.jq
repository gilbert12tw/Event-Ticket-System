(.data.result // [])
| if length == 0 then
    "- no samples"
  else
    .[]
    | "- \(.metric.instance // .metric.service // "unknown"): `\(.value[1] // "n/a")`"
  end
