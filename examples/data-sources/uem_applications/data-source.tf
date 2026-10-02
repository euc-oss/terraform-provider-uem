data "uem_applications" "example" {
  name     = "Example Application"
  platform = "Apple"
}

output "example_application_ids" {
  value = data.uem_applications.example.applications[*].id
}
