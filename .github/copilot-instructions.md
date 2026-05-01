# GitHub Copilot Instructions
When reviewing a pull request, please ensure it adheres strictly to the instructions in the Rules/Coding Guidelines section of CONTRIBUTING.md, if it exists.
## Code Style
- Follow the established code style in existing files
- Use meaningful variable and function names that describe their purpose
- Prefer clarity over brevity unless working with well-established idioms
- Use consistent indentation and formatting
- Keep functions and methods focused on a single responsibility
- Avoid unnecessary comments; write self-documenting code when possible
## Commit message policy (MUST FOLLOW)
For any Git commit you create in this repository (including intermediate "WIP" commits):

- Prefix the commit subject line with: `[Automation]`
- After the prefix, write a concise summary between 16 and 62 characters.
- Do not include a newline in the subject line.
- If a commit body is included, add it after a blank line and ensure it is at least 32 characters.

### Examples
- `[Automation] Fix null pointer in user profile mapping`
- `[Automation] Add retries to API client request handling`

If you cannot comply with these commit-message rules, stop and ask for guidance before committing.
## Documentation
- Document public interfaces with clear descriptions of parameters, return values, and exceptions
- Include examples for complex or non-obvious functionality
- Explain "why" rather than "what" in comments
- Keep documentation up-to-date with code changes
## Error Handling
- Handle errors explicitly rather than relying on default behaviors
- Use appropriate error handling mechanisms for the language
- Provide meaningful error messages
- Fail fast and visibly when encountering unexpected conditions
## Security
- Never hardcode credentials, API keys, or secrets in source code
- Validate all user inputs before processing
- Use secure, up-to-date libraries and frameworks
- Apply the principle of least privilege
- Consider potential security implications of generated code
## Performance
- Optimize for readability and maintainability first, then performance
- Consider time and space complexity for algorithms
- Avoid premature optimization
- Use appropriate data structures for the task at hand
## Testing
- Write testable code
- Include unit tests for new functionality
- Cover edge cases and error conditions
- Use descriptive test names that explain the expected behavior
## Project Structure
- Place new code in appropriate modules/directories
- Follow the established project organization
- Reuse existing utilities and helpers rather than duplicating code
- Keep related functionality together
## Naming Conventions
- Use consistent naming patterns across the codebase
- Follow language-specific naming conventions when applicable
- Make names descriptive and unambiguous
- Use domain-specific terminology where appropriate
## Dependencies
- Minimize external dependencies
- Use well-maintained and actively supported libraries
- Consider compatibility and licensing when suggesting dependencies
## Additional Guidelines
- Maintain backward compatibility when modifying existing interfaces
- Add proper logging for important events and error conditions
- Consider internationalization where appropriate
- Write code that is accessible and inclusive
# Language-Specific Guidelines
## Go
---
applyTo:
  - "**/*.go"
  - "**/go.mod"
  - "**/go.sum"
---
- Follow the official Go style guidelines (gofmt)
- Use MixedCaps or mixedCaps for naming according to visibility (exported or not)
- Return errors as the last return value and check them explicitly
- Prefer composition over inheritance using embedding
- Use meaningful package names that reflect their purpose
- Keep interfaces small and focused
- Use goroutines and channels for concurrent operations
- Handle errors explicitly. Don't use panic for normal error handling
- Use defer for resource cleanup
- Utilize the standard library when possible
- Include documentation comments for exported functions and types
- Write table-driven tests using the testing package
- Organize code by feature rather than by layer
- Use context for cancellation and request-scoped values
- Follow the principle "Accept interfaces, return structs"