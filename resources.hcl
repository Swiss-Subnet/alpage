# Shared registry entities: the subnet, data centers, and node providers.
# Operators and their nodes live one-file-per-operator under resources/; all
# files are merged, so references resolve across them
# (e.g. node_operator.solnet_so1.provider = node_provider.solnet.id).
# Referenced as <kind>.<name>.id from proposals and from each other.
# Hierarchy: node -> node_operator -> node_provider, and node_operator -> data_center.
# `alp reconcile` diffs nodes against live subnet membership and operator
# ownership; provider/operator/dc facts are from the registry canister
# (get_node_operators_and_dcs_of_node_provider).

subnet "swiss" {
  id    = "3zsyy-cnoqf-tvlun-ymf55-tkpca-ox7uw-kfxoh-7khwq-2gz43-wafem-lqe"
  label = "Swiss Subnet"
}

data_center "so1" {
  id     = "so1"
  region = "Europe,CH,Solothurn"
}

data_center "fl2" {
  id     = "fl2"
  region = "Europe,LI,Vaduz"
}

data_center "ag1" {
  id     = "ag1"
  region = "Europe,CH,Aargau"
}

data_center "ne1" {
  id     = "ne1"
  region = "Europe,CH,Fribourg"
}

data_center "fl1" {
  id     = "fl1"
  region = "Europe,LI,Eschen"
}

data_center "vs1" {
  id     = "vs1"
  region = "Europe,CH,Valais"
}

data_center "vd1" {
  id     = "vd1"
  region = "Europe,CH,Vaud"
}

data_center "fl3" {
  id     = "fl3"
  region = "Europe,LI,Schaan"
}

data_center "lu1" {
  id     = "lu1"
  region = "Europe,CH,Luzern"
}

data_center "sz1" {
  id     = "sz1"
  region = "Europe,CH,Zug"
}

data_center "gr1" {
  id     = "gr1"
  region = "Europe,CH,Graubünden"
}

data_center "ti1" {
  id     = "ti1"
  region = "Europe,CH,Ticino"
}

data_center "ai1" {
  id     = "ai1"
  region = "Europe,CH,Appenzell Ausserrhoden"
}

node_provider "solnet" {
  id    = "mf6om-4m4yc-36jur-ip35a-6d3yr-kqi7v-txofz-nraz3-f6a4l-dcufx-oqe"
  label = "SolNet"
}

node_provider "vestra" {
  id    = "izdfy-ocmaz-3qwcy-lluqx-tvq64-oybib-oyhxx-3dfni-ssznb-suhes-iqe"
  label = "vestra ICT AG"
}

node_provider "swiss_datalink" {
  id    = "hycj4-e3jwh-l2bqz-ohuxh-tu4af-agzov-uugg6-j57rk-b6opc-fx3ml-kqe"
  label = "Swiss Datalink AG"
}

node_provider "senselan" {
  id    = "f5kd2-ylls6-e6cts-6exqp-pwra3-djn2g-lnvbi-a3qs6-cfdr6-ti5dw-qqe"
  label = "senseLAN"
}

node_provider "ltin" {
  id    = "qpwbv-tu7uj-vpndf-talpd-zufus-itpe5-n66ua-yahhs-ttiu5-ileoc-2qe"
  label = "LTIN AG"
}

node_provider "aitubi" {
  id    = "znw2p-4cx6u-ocqls-277iu-2lkir-xjy7g-4s3sj-sjy6j-mtlay-rnnra-yqe"
  label = "Aitubi AG"
}

node_provider "alpinedc" {
  id    = "mrfhx-rsvqz-jndwd-3nrkb-fw3wy-cq64z-iszxt-drffc-f4rtj-ivoop-6ae"
  label = "AlpineDC SA"
}

node_provider "decentralized" {
  id    = "hokzb-gsg3k-oj44m-tqnhs-mpmwl-ujv4x-44bsz-gdoce-pl6tv-oin7v-eae"
  label = "Decentralized"
}

node_provider "achermann" {
  id    = "gjsts-tuec7-wp6cl-zmk6w-sfpp6-ei34c-l7njq-les4c-yupv3-hbcpg-tae"
  label = "achermann.swiss"
}

node_provider "coreledger" {
  id    = "g4gfo-2buho-hg3ho-pamsx-yg2vz-qnz2r-fsn65-j6dv7-myful-iy6vv-tqe"
  label = "CoreLedger"
}

node_provider "big" {
  id    = "c3i3u-4ot4i-zino3-jrxre-7s426-dk2th-cvino-nznco-lkbpn-vl5of-4qe"
  label = "Blockchain Innovation Group"
}

node_provider "noku" {
  id    = "64kb5-mzfmq-5wq5v-tm4p6-hekel-ne5xb-amiwr-cwvgg-t7db6-jlpau-nae"
  label = "NOKU SA"
}

node_provider "avalution" {
  id    = "is2tg-4for6-ytyzl-5xokl-jd3kz-4y5ky-g7am2-yotrq-5yruf-twke6-vae"
  label = "Avalution AG"
}
