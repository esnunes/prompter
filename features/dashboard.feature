Feature: Dashboard
  As a user
  I want to manage my GitHub repositories from a central dashboard
  So that I can create and track prompt requests across projects

  Background:
    Given I am on the dashboard page

  # --- Page Layout ---

  Scenario: Dashboard displays the page title
    Then the page title should be "Prompter — Dashboard"

  Scenario: Dashboard displays the sidebar with all prompt requests
    Given a repository "github.com/acme/web" exists
    And a prompt request "Fix login bug" exists for "github.com/acme/web"
    And a repository "github.com/acme/api" exists
    And a prompt request "Add caching" exists for "github.com/acme/api"
    Then the sidebar should display "Fix login bug"
    And the sidebar should display "Add caching"
    And each sidebar item should display the repository name

  Scenario: Dashboard displays the "Add repository" form
    Then I should see a form labeled "Add repository"
    And the form should have an input with placeholder "github.com/owner/repo"
    And the form should have a submit button labeled "Add"

  # --- Repository List ---

  Scenario: Empty dashboard shows getting started message
    Then I should see "No repositories yet"
    And I should see "Enter a repository URL above to get started."

  Scenario: Dashboard lists repositories with prompt requests
    Given a repository "github.com/acme/web" exists
    And a prompt request "Fix login bug" exists for "github.com/acme/web"
    Then I should see "github.com/acme/web" in the repository list
    And the repository card should show "1 prompt requests"

  Scenario: Newly added repository with no prompt requests is visible
    Given a repository "github.com/acme/new" exists
    And no prompt requests exist for "github.com/acme/new"
    Then I should see "github.com/acme/new" in the repository list

  Scenario: Repository cards link to the repository detail page
    Given a repository "github.com/acme/web" exists
    And a prompt request exists for "github.com/acme/web"
    Then the repository card for "github.com/acme/web" should link to "/github.com/acme/web/prompt-requests"

  Scenario: Repositories are sorted by last activity
    Given a repository "github.com/acme/old" exists with last activity "2026-01-01"
    And a repository "github.com/acme/new" exists with last activity "2026-03-15"
    Then "github.com/acme/new" should appear before "github.com/acme/old" in the repository list

  Scenario: Deleted prompt requests are not counted
    Given a repository "github.com/acme/web" exists
    And a prompt request "Active" exists for "github.com/acme/web" with status "draft"
    And a prompt request "Removed" exists for "github.com/acme/web" with status "deleted"
    Then the repository card should show "1 prompt requests"

  Scenario: Archived prompt requests are not counted as active
    Given a repository "github.com/acme/web" exists
    And a prompt request "Active" exists for "github.com/acme/web"
    And an archived prompt request "Old" exists for "github.com/acme/web"
    Then the repository card should show "1 prompt requests"

  # --- Add Repository: URL Sanitization ---

  Scenario Outline: Form sanitizes common URL formats
    When I enter "<input>" in the repository URL field
    And I submit the form
    Then the repository "github.com/owner/repo" should be created
    And I should be redirected to "/github.com/owner/repo/prompt-requests"

    Examples:
      | input                                |
      | github.com/owner/repo                |
      | https://github.com/owner/repo        |
      | http://github.com/owner/repo         |
      | github.com/owner/repo/               |
      | github.com/owner/repo.git            |
      | https://github.com/owner/repo.git    |
      | https://github.com/owner/repo.git/   |
      |   github.com/owner/repo              |

  # --- Add Repository: Validation ---

  Scenario: Submitting an empty URL shows validation error
    When I submit the form with an empty repository URL
    Then the form should re-render with an error
    And I should see "Invalid repository URL"
    And I should not be redirected

  Scenario Outline: Submitting an invalid URL shows validation error
    When I enter "<input>" in the repository URL field
    And I submit the form
    Then the form should re-render with an error
    And I should see "Invalid repository URL"
    And the input should preserve my original text "<input>"

    Examples:
      | input              |
      | not-a-url          |
      | github.com/owner   |
      | gitlab.com/o/r     |
      | github.com/        |

  Scenario: Submitting a URL for a non-existent GitHub repository shows error
    Given the GitHub repository "owner/nonexistent" does not exist
    When I enter "github.com/owner/nonexistent" in the repository URL field
    And I submit the form
    Then the form should re-render with an error
    And I should see "doesn't exist on GitHub"
    And the input should preserve my original text "github.com/owner/nonexistent"

  Scenario: Form preserves user's original input on validation error
    When I enter "https://github.com/bad url/repo" in the repository URL field
    And I submit the form
    Then the input should preserve my original text "https://github.com/bad url/repo"

  Scenario: Form preserves user's original input on GitHub verification error
    Given the GitHub repository "owner/private" does not exist
    When I enter "https://github.com/owner/private.git" in the repository URL field
    And I submit the form
    Then the input should preserve my original text "https://github.com/owner/private.git"

  # --- Add Repository: Success ---

  Scenario: Successfully adding a new repository
    Given the GitHub repository "acme/web" exists
    When I enter "github.com/acme/web" in the repository URL field
    And I submit the form
    Then the repository "github.com/acme/web" should be created in the database
    And I should be redirected to "/github.com/acme/web/prompt-requests"

  Scenario: Adding the same repository twice is idempotent
    Given the GitHub repository "acme/web" exists
    And a repository "github.com/acme/web" already exists in the database
    When I enter "github.com/acme/web" in the repository URL field
    And I submit the form
    Then I should be redirected to "/github.com/acme/web/prompt-requests"
    And no duplicate repository should be created

  # --- HTMX Behavior ---

  Scenario: Add repository form submits via HTMX
    Then the form should post to "/hx/dashboard/create-repository"
    And the form should swap its own content on response

  Scenario: Successful submission redirects via HX-Location
    Given the GitHub repository "acme/web" exists
    When I submit the form with "github.com/acme/web"
    Then the response should include an "HX-Location" header
    And the "HX-Location" value should be "/github.com/acme/web/prompt-requests"

  Scenario: Repository cards use hx-boost for SPA navigation
    Given a repository "github.com/acme/web" exists with prompt requests
    Then the repository card links should have hx-boost enabled
