# Inline-id variant of swiss-subnet-wave1, with no resources.hcl alongside.
# Used to prove inline ids and resource references produce the same payload.
proposal "swiss-subnet-wave1" {
  kind    = "resize"
  title   = "Resize the Swiss Subnet for the SEV migration (wave 1)"
  summary = "Remove 6 nodes (13 -> 7) to be redeployed with SEV-SNP enabled as the first wave of the confidential-computing migration."
  url     = "https://forum.dfinity.org/t/migrating-the-swiss-subnet-to-confidential-computing-sev-snp/74619"

  resize {
    subnet_id = "3zsyy-cnoqf-tvlun-ymf55-tkpca-ox7uw-kfxoh-7khwq-2gz43-wafem-lqe"

    remove { id = "eihqt-opds6-5r7xa-hr32z-k2xoa-bh4l4-65kld-v7gk3-bplt5-vmsw2-mqe" }
    remove { id = "ezsx4-peoff-6kofj-yz6vt-gc42v-iugvx-vit2r-edy37-qv4bt-ivcxy-kae" }
    remove { id = "g5s3p-l63zo-lsigy-5u4t5-476io-5m62n-5qmuo-pjgv5-pnq4n-uhxhp-7qe" }
    remove { id = "lemsa-bnpvg-zzzcq-6uwar-njtds-byn3n-zcb7v-du25b-tktfx-32gc5-zae" }
    remove { id = "rumwa-ihvrd-kqzmq-4ainw-birq6-2y4v7-sbqyx-lw3vm-osdhm-nh5hl-cae" }
    remove { id = "vou34-3jw7y-l2tah-tssac-y5xop-3x3q4-z2ivi-gvmoc-amv7t-akkbz-vae" }
  }
}
