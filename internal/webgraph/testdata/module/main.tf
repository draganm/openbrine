resource "test_vpc" "main" {
  cidr_block = "10.0.0.0/16"
}

module "net" {
  source     = "./mod"
  vpc_id     = test_vpc.main.id
  cidr_block = test_vpc.main.cidr_block
}

output "subnet_id" {
  value = module.net.subnet_id
}
