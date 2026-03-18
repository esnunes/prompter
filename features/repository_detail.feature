Feature: Repository Detail
  As a user
  I want to view and manage prompt requests for a specific repository
  So that I can track and organize my AI-guided conversations

  # --- Page Layout ---

  Scenario: Page displays the repository URL as title
    Given a repository "github.com/acme/web" exists
    When I visit "/github.com/acme/web/prompt-requests"
    Then the page title should be "github.com/acme/web — Prompter"
    And I should see "github.com/acme/web" as the page heading

  Scenario: Page displays a "New prompt request" button in the header
    Given a repository "github.com/acme/web" exists
    When I visit "/github.com/acme/web/prompt-requests"
    Then I should see a "New prompt request" button in the header

  Scenario: Page displays a "Show archived" toggle
    Given a repository "github.com/acme/web" exists
    When I visit "/github.com/acme/web/prompt-requests"
    Then I should see a "Show archived" checkbox
    And the checkbox should be unchecked

  # --- Repository Not Found ---

  Scenario: Visiting a repository that is not in the database shows an error
    Given no repository "github.com/acme/unknown" exists in the database
    When I visit "/github.com/acme/unknown/prompt-requests"
    Then I should see "Repository not found"
    And I should see "Add it from the dashboard first."
    And I should see a "Back to dashboard" link pointing to "/"

  Scenario: "New prompt request" button is hidden when repository is not found
    Given no repository "github.com/acme/unknown" exists in the database
    When I visit "/github.com/acme/unknown/prompt-requests"
    Then I should not see a "New prompt request" button

  # --- Prompt Request List ---

  Scenario: Empty repository shows getting started message
    Given a repository "github.com/acme/web" exists
    And no prompt requests exist for "github.com/acme/web"
    When I visit "/github.com/acme/web/prompt-requests"
    Then I should see "No prompt requests yet"
    And I should see "Create your first prompt request"

  Scenario: Repository lists its prompt requests
    Given a repository "github.com/acme/web" exists
    And a prompt request "Fix login bug" exists for "github.com/acme/web" with status "draft"
    And a prompt request "Add caching" exists for "github.com/acme/web" with status "published"
    When I visit "/github.com/acme/web/prompt-requests"
    Then I should see a card for "Fix login bug" with a "draft" badge
    And I should see a card for "Add caching" with a "published" badge

  Scenario: Prompt request card shows metadata
    Given a repository "github.com/acme/web" exists
    And a prompt request "Fix login bug" exists for "github.com/acme/web"
    And the prompt request has 5 messages and 2 revisions
    When I visit "/github.com/acme/web/prompt-requests"
    Then the card should show "5 messages"
    And the card should show "2 revisions"
    And the card should show the creation date

  Scenario: Prompt request card with no revisions hides revision count
    Given a repository "github.com/acme/web" exists
    And a prompt request "New feature" exists for "github.com/acme/web"
    And the prompt request has 0 revisions
    When I visit "/github.com/acme/web/prompt-requests"
    Then the card should not show a revision count

  Scenario: Prompt request without a title shows "Untitled"
    Given a repository "github.com/acme/web" exists
    And a prompt request with no title exists for "github.com/acme/web"
    When I visit "/github.com/acme/web/prompt-requests"
    Then I should see a card labeled "Untitled"

  Scenario: Prompt request card links to the conversation page
    Given a repository "github.com/acme/web" exists
    And a prompt request with ID 42 exists for "github.com/acme/web"
    When I visit "/github.com/acme/web/prompt-requests"
    Then the card should link to "/github.com/acme/web/prompt-requests/42"

  Scenario: Deleted prompt requests are not shown
    Given a repository "github.com/acme/web" exists
    And a prompt request "Active" exists for "github.com/acme/web" with status "draft"
    And a prompt request "Removed" exists for "github.com/acme/web" with status "deleted"
    When I visit "/github.com/acme/web/prompt-requests"
    Then I should see a card for "Active"
    And I should not see a card for "Removed"

  Scenario: Archived prompt requests are not shown by default
    Given a repository "github.com/acme/web" exists
    And a prompt request "Active" exists for "github.com/acme/web"
    And an archived prompt request "Old stuff" exists for "github.com/acme/web"
    When I visit "/github.com/acme/web/prompt-requests"
    Then I should see a card for "Active"
    And I should not see a card for "Old stuff"

  # --- Archive Toggle ---

  Scenario: Checking "Show archived" displays only archived prompt requests
    Given a repository "github.com/acme/web" exists
    And a prompt request "Active" exists for "github.com/acme/web"
    And an archived prompt request "Old stuff" exists for "github.com/acme/web"
    When I visit "/github.com/acme/web/prompt-requests?archived=1"
    Then I should see a card for "Old stuff"
    And I should not see a card for "Active"
    And the "Show archived" checkbox should be checked

  Scenario: No archived prompt requests shows empty message
    Given a repository "github.com/acme/web" exists
    And no archived prompt requests exist for "github.com/acme/web"
    When I visit "/github.com/acme/web/prompt-requests?archived=1"
    Then I should see "No archived prompt requests"
    And I should see "You haven't archived any prompt requests for this repository."

  Scenario: Unchecking "Show archived" returns to the active list
    Given a repository "github.com/acme/web" exists
    When I am viewing archived prompt requests at "/github.com/acme/web/prompt-requests?archived=1"
    And I uncheck the "Show archived" checkbox
    Then the page should navigate to "/github.com/acme/web/prompt-requests"

  Scenario: Checking "Show archived" updates the URL
    Given a repository "github.com/acme/web" exists
    When I visit "/github.com/acme/web/prompt-requests"
    And I check the "Show archived" checkbox
    Then the URL should change to "/github.com/acme/web/prompt-requests?archived=1"

  # --- Archive / Unarchive Actions ---

  Scenario: Active prompt requests have an archive action
    Given a repository "github.com/acme/web" exists
    And a prompt request "Fix login bug" exists for "github.com/acme/web"
    When I visit "/github.com/acme/web/prompt-requests"
    Then each card should have an "Archive prompt" action

  Scenario: Archiving a prompt request asks for confirmation
    Given a repository "github.com/acme/web" exists
    And a prompt request "Fix login bug" exists for "github.com/acme/web"
    When I visit "/github.com/acme/web/prompt-requests"
    And I click the archive action on "Fix login bug"
    Then I should be asked to confirm "Archive this prompt request?"

  Scenario: Confirming archive removes the card from the list
    Given a repository "github.com/acme/web" exists
    And a prompt request "Fix login bug" exists for "github.com/acme/web"
    When I visit "/github.com/acme/web/prompt-requests"
    And I archive "Fix login bug" and confirm
    Then the card for "Fix login bug" should be removed from the page

  Scenario: Archived prompt requests have an unarchive action
    Given a repository "github.com/acme/web" exists
    And an archived prompt request "Old stuff" exists for "github.com/acme/web"
    When I visit "/github.com/acme/web/prompt-requests?archived=1"
    Then each card should have an "Unarchive prompt" action

  Scenario: Unarchiving a prompt request asks for confirmation
    Given a repository "github.com/acme/web" exists
    And an archived prompt request "Old stuff" exists for "github.com/acme/web"
    When I visit "/github.com/acme/web/prompt-requests?archived=1"
    And I click the unarchive action on "Old stuff"
    Then I should be asked to confirm "Unarchive this prompt request?"

  Scenario: Confirming unarchive removes the card from the archived list
    Given a repository "github.com/acme/web" exists
    And an archived prompt request "Old stuff" exists for "github.com/acme/web"
    When I visit "/github.com/acme/web/prompt-requests?archived=1"
    And I unarchive "Old stuff" and confirm
    Then the card for "Old stuff" should be removed from the page

  # --- New Prompt Request ---

  Scenario: Clicking "New prompt request" creates one and redirects to conversation
    Given a repository "github.com/acme/web" exists
    When I visit "/github.com/acme/web/prompt-requests"
    And I click "New prompt request"
    Then a new prompt request should be created for "github.com/acme/web"
    And I should be redirected to the new prompt request's conversation page

  # --- HTMX Behavior ---

  Scenario: Prompt request card links use hx-boost for SPA navigation
    Given a repository "github.com/acme/web" exists with prompt requests
    When I visit "/github.com/acme/web/prompt-requests"
    Then the prompt request card links should have hx-boost enabled

  Scenario: Archive toggle updates the URL without full page reload
    Given a repository "github.com/acme/web" exists
    When I visit "/github.com/acme/web/prompt-requests"
    Then the "Show archived" checkbox should push the URL on change

  Scenario: "Back to dashboard" link uses hx-boost for SPA navigation
    Given no repository "github.com/acme/unknown" exists in the database
    When I visit "/github.com/acme/unknown/prompt-requests"
    Then the "Back to dashboard" link should have hx-boost enabled
