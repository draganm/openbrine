resource "test_vpc" "main" {
  cidr_block = "10.0.0.0/16"
}

resource "test_subnet" "direct" {
  vpc_id     = test_vpc.main.id
  cidr_block = test_vpc.main.cidr_block
}

resource "test_subnet" "transformed" {
  vpc_id     = test_vpc.main.id
  cidr_block = cidrsubnet(test_vpc.main.cidr_block, 4, 1)
  name       = "${test_vpc.main.id}-suffix"
}

data "test_lookup" "info" {
  vpc_id = test_vpc.main.id
}

output "subnet_id" {
  value = test_subnet.direct.id
}

# A for-expression iteration variable (k) must NOT produce a phantom
# edge to a non-existent resource named "k".
output "id_map" {
  value = { for k, v in { a = test_subnet.direct, b = test_subnet.transformed } : k => v.id }
}
