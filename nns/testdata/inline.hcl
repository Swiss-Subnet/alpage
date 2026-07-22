# Literal-id twin of config.hcl's resize-example; must hash identically.
proposal "resize-example" {
  kind  = "resize"
  title = "Resize fixture"

  resize {
    subnet_id = "67htk-vfkxp-gn33q-baibq"
    remove { id = "5ffj3-jarcq-lruhj-aemtc-sla" }
  }
}
