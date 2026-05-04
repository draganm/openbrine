variable "vpc_id" {
  type = string
}

variable "cidr_block" {
  type = string
}

resource "test_subnet" "child" {
  vpc_id     = var.vpc_id
  cidr_block = var.cidr_block
}

output "subnet_id" {
  value = test_subnet.child.id
}
