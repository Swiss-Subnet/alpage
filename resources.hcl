# Named resources for the Swiss Subnet, referenced from proposals via HCL
# expressions: node.<name>.id and subnet.<name>.id. Names must be valid HCL
# identifiers (letters, digits, underscore). ids resolved from the registry.

subnet "swiss" {
  id    = "3zsyy-cnoqf-tvlun-ymf55-tkpca-ox7uw-kfxoh-7khwq-2gz43-wafem-lqe"
  label = "Swiss Subnet"
}

node "achermann_luzern_1" {
  id    = "eihqt-opds6-5r7xa-hr32z-k2xoa-bh4l4-65kld-v7gk3-bplt5-vmsw2-mqe"
  label = "achermann.swiss, Luzern 1"
}

node "coreledger_zug_1" {
  id    = "ezsx4-peoff-6kofj-yz6vt-gc42v-iugvx-vit2r-edy37-qv4bt-ivcxy-kae"
  label = "CoreLedger, Zug 1"
}

node "alpinedc_vaud_1" {
  id    = "g5s3p-l63zo-lsigy-5u4t5-476io-5m62n-5qmuo-pjgv5-pnq4n-uhxhp-7qe"
  label = "AlpineDC SA, Vaud 1"
}

node "big_graubunden_1" {
  id    = "lemsa-bnpvg-zzzcq-6uwar-njtds-byn3n-zcb7v-du25b-tktfx-32gc5-zae"
  label = "Blockchain Innovation Group, Graubunden 1"
}

node "noku_ticino_1" {
  id    = "rumwa-ihvrd-kqzmq-4ainw-birq6-2y4v7-sbqyx-lw3vm-osdhm-nh5hl-cae"
  label = "NOKU SA, Ticino 1"
}

node "avalution_appenzell_1" {
  id    = "vou34-3jw7y-l2tah-tssac-y5xop-3x3q4-z2ivi-gvmoc-amv7t-akkbz-vae"
  label = "Avalution AG, Appenzell Ausserrhoden 1"
}
