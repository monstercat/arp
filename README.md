# Arp
Short for Arpeggio, is a tool for automating **REST** JSON API calls that can be used for integration tests, simulation, checks, etc.

## About
This tool provides a lightweight black-box testing framework for your REST API. 
All you need to provide are your inputs to the route and a validation definition for the response. 
The tool will then execute the requests, run your validation against the response, and then print a pretty report at the end. All tests are
written as YAML configs so no coding is required.

### The Inspiration
If you've ever had to write a backend API, you have probably written curl commands or configured PostMan to execute calls against your API for testing purposes. In this testing process, you have probably encountered one or more of the following scenarios:
1. You don't care about the exact details of the API response, but are only looking to validate the shape of it
2. You want to perform an exact match on the API response with a known good response
3. Your API call depends on the response of another API call that you have to manually copy and paste into the input of your current test
4. Repeat 3 as many times as it takes to get to the API state you are trying to validate.

This program helps solve those problems by allowing you to:
* Define validations based on the shape of the response
* Perform exact or fuzzy matching on values within the response
* Filter for items in the response and pick out specific data to use as an input for future test cases

It basically tries to automate the process for why you are writing a curl command or setting PostMan up for in the first place. If you were going through the effort to use curl or PostMan for validation, it is 
not much more effort to extend that into a repeatable test case.

### Definitions
* **Test:** An individual test case that provides an input and validates a response
* **Test Suite:** A file containing multiple tests
* **Validation Definition:** A structure containing rules on how to validate a specific response property
* **Validation Matcher:** A comparison value/placeholder to evaluate against a primitive data type within the response (e.g. bool, string, int) 

### Features
* **Test definitions as Config:** Use yaml to define your tests. It's easy to write and less verbose than JSON. You can use anchoring to reuse sections or values in multiple places.
* **Data Persistence:** You can store results that your validation matches and use them as inputs or matchers in subsequent tests
* **Interactive Mode:** You can run test files in an interactive mode allowing you to retry individual tests, evaluate variables, or dump the data in the data store at that point in the test suite's execution

## Installation
```
git clone https://github.com/monstercat/arp.git
cd arp
go build ./cmd/arp
```

## Usage
```text
Usage of ./arp:
  -colors
        Print test report with colors (default true)
  -file string
        Single file path to a test suite to execute.
  -fixtures string
        Path to yaml file with data to include into the test scope via test variables. (default "./fixtures.yaml")
  -short
        Print a short report for executed tests containing only the validation results (default true)
  -short-fail
        Keep the report short when errors are encountered rather than expanding with details
  -step
        Execute a single test file in interactive mode. Requires a test file to be provided with '-file'
  -test-root string
        File path to scan and execute test files from
  -threads int
        Max number of test files to execute concurrently (default 16)
  -tiny
        Print an even tinier report output than what the short flag provides. Only prints test status, name, and description. Failed tests will still be expanded
  -var value
        Prepopulate the tests data store with a KEY=VALUE pair.
```


## Sample Test
Here is a simple test case that can be executed

```yaml
# sample.yaml
tests:
  - name: "List Users"
    description: "Listing all users"
    route: "https://reqres.in/api/users"
    method: GET
    response:
      code: 200
      payload:
        page:
          type: integer
          matches: $any
        per_page:
          type: integer
          matches: $any
        total:
          type: integer
          matches: $any
        total_pages:
          type: integer
          matches: $any
        data:
          type: array
          length: $notEmpty
          items:
            - type: object
              properties:
                id:
                  type: integer
                  matches: $any
                email:
                  type: string
                  matches: $any
                first_name:
                  type: string
                  matches: $any
                last_name:
                  type: string
                  matches: $any
                avatar:
                  type: string
                  matches: https://.*
```

Sample output:

![Sample Output](./.github/images/demo.gif)


All tests in a directory can be executed with the `-test-root` flag like so:

```./arp -test-root="<path to directory containing foo_test.yaml>"```

Individual files can be executed using the `-file` flag:

```./arp -file=<path>/foo_test.yaml```






## Validations

Each JSON data type has its own set of validation rules that can be applied.

### Integers
```yaml
payload:
  MyInteger:
    type: integer
    matches: <matcher>
```

Supported matchers:
* a specific integer value: e.g. -1, 0, 1, 2, 3, ...
* The **$any** key word to match regardless of the numerical value

### Numbers
```yaml
payload:
  MyNumber:
    type: number
    matches: <matcher>
```

Supported matchers:
* a specific numerical value: e.g. 1, 2, 3, 3.14, etc.
* The **$any** key word to match regardless of the numerical value

### Booleans
```yaml
payload:
  MyBool:
    type: bool
    matches: <matcher>
```

Supported matchers:
* true or false
* The **$any** keyword to match regardless of the numerical value

### Strings
```yaml
payload:
  MyString:
    type: string
    matches: <matcher>
```

Supported matchers:
* any regular expression: e.g. ".*", "SomePartial.+", "[0-9]+"
* The **$any** keyword to match any string (".*" expression)
* The **$notEmpty** keyword to match non-empty strings (".+" expression)

### Arrays
```yaml
payload:
  MyArray:
    type: array
    length: <matcher>
    items:
      - <sub validations>
```

Supported matchers (for length):
* any integer value: e.g. 1, 2, 200, ...
* The **$notEmpty** keyword
* Length expressions like: **$< 5**, **$<= 1**, **$> 1**, **$>= 500**


Array validation performs a length check independent of the item validations provided to it. This means you can validate that your array has returned a length of X
but then only define a validator for the first element within it with the assumption that the rest of the items are the same.

For small arrays, you can validate that all the items are the same by simply repeating the validation for X number of expected items. E.g:
```yaml
payload:
  MyArray:
    type: array
    length: 2
    items:
      # validate that each item returned is a string
      - type: string
        matches: $any
      - type: string
        matches: $any
```

You can set the `sorted` property to false in the event that you are validating a large array response and are looking to seek out a specific item from it. This will have the validation
perform a depth first search for the first node the validation matches on.


```yaml
payload:
  MyArray:
    length: $notEmpty
    sorted: false
    items:
      # validate that array contains an object that has a Name property with `MySpecificPerson`
      - type: object
        properties:
          Name:
            type: string
            matches: MySpecificPerson

```

This mechanism can be used to 'pick' values from the response to put into the data store for use later. However, you need to ensure the most specific validation is executed first to locate the correct node. Otherwise
you may get an unexpected node that the more general validation matched on first.

```yaml
# Get the ID for MySpecificPerson and store it in the data store for a future test
payload:
  MyArray:
    length: $notEmpty
    sorted: false
    items:
      - type: object
        properties:
          Name:
            # Run this validation at a high priority than the ID since we are locating the ID based on the name
            priority: 0
            type: string
            matches: MySpecificPerson
          Id:
            # ID matcher is more generic (since we don't know the ID before hand). It needs to be executed AFTER
            # the name validation which will be used to locate the specific node in our unsorted response.
            priority: 1
            type: string
            matches: $any
            # subsequent tests can now reference the ID associated to 'MySpecificPerson' through the @{test_id} variable
            storeAs: test_id

```

### Objects
```yaml
payload:
  MyObject:
    type: object
    properties:
      FieldOne: <sub validation>
```

All objects are validated based on the keys present in the validations definition. If `MyObject` exists and the `FieldOne` property does not exist, a validation error will be raised.

### Field Existence
If you want to validate that a field `does not exist`, you can add the `exists` property to your validation:
```yaml
payload:
  MyObject:
    type: object
    properties:
      FieldOne:
        type: string
        matches: $any
      FieldTwo:
        type: string
        # if the FieldTwo key exists, this will raise a validation error
        exists: false
```




## Data Storage

Each *Test Suite* has its own isolated data store that the tests can read and write variables to. Variables are read using `@{myVarName}` notation, and are
saved using the `storeAs` property on your field matcher.  

Variables can only be referenced in the following test definition fields:
* input
```yaml
tests:
  - name: Something
    input:
       api_token: @{MY_API_KEY}
---
```


* headers
```yaml
tests:
  - name: Something
    headers:
      SomeHeader: @{headerStuff}
---
```

* route
```yaml
tests:
  - name: Something
    route: '@{host}/user/@{user_id}'
---
```

* validation `matches` fields as strings
```yaml
---
payload:
  Name:
    type: string
    matches: '@{name}'
---
```


For example, if we wanted to store the ID  of the user Charles from the sample test to use in a subsequent GET call. Our test would look like:

```yaml
#sample.yaml
  - name: Get Charles ID
    description: Get the ID for user charles
    route:  https://reqres.in/api/users
    method: GET
    response:
      code: 200
      payload:
        data:
          type: array
          length: $notEmpty
          sorted: false
          items:
            - type: object
              properties:
                email:
                  # set email to a higher priority than ID so this matcher executes first to pick out our array element
                  priority: 0
                  type: string
                  # create an anchor here for the email so we don't have to type it out again
                  matches: &charles_email 'charles.morris@reqres.in'
                id:
                  # id matcher is set to a lower priority than email. This matcher is as generic as can be and can pick out the wrong node
                  # if evaluated first.
                  priority: 1
                  type: integer
                  matches: $any
                  # store the value in a variable called 'charles_id'
                  storeAs: charles_id
  - name: Get Charles Data
    description: Get the data for user Charles
    # Fetch the user using the ID we got from the previous test
    route: https://reqres.in/api/users/@{charles_id}
    method: GET
    response:
      code: 200
      payload:
        data:
          type: object
          properties:
            email:
              type: string
              # proof that we got the right user
              matches: *charles_email
```

![DFS Store](./.github/images/demo2.gif)


### Fixtures

The data store can be pre-populated with read only data prior to executing your tests with a file containing data definitions. This file should consist of a maps that terminate to
a string value. E.g.

```yaml
#fixtures.yaml
Hosts:
  Beta: 
    Local: http://localhost
    RealBeta: http://beta.NotLocalHost.com
  Prod: http://NotLocalHost.com

# foo_test.yaml
tests:
  - name: List Foos
    # Access our host using the fixture mapping with dot ('.') notation
    route: '@{Hosts.Beta.Local}/foo?search=Bar'
```

### Environment Variables

The data store will also be pre-populated with your system's environment variables and can be access the same way as any other variable

```shell
# Env var
export MY_API_TOKEN="adfadfadfadfa"
```

```yaml
# foo_test.yaml
tests:
  - name: List Foos
    input:
      # populate our apiToken input field with our environment variable value
      apiToken: @{MY_API_TOKEN}
    ...
```

### Var Parameters
Alternatively, you can provide variables with one or more `-var` input parameters following the `KEY=VALUE` syntax:

```bash
./arp -test-root=. -var='MY_API_TOKEN=adfadfadfadfa' -var='SOMETHING_ELSE=not the token'
```


### Variable Composition
Variables can also be composed of other variables, or in other words, you can have a variable in the name of a variable:

```yaml
'@{Hosts.@{HOST_STAGE}}/foo'
```

Variables are resolved at run time starting from the most nested variable. If we had the following fixture file
```yaml
# fixtures.yaml
Hosts:
    Beta: http://localhost
    Prod: http://NotLocalHost.com
```

And we set the HOST_STAGE to beta as an environment variable:
```shell
#Env Var
export HOST_STAGE="Beta"
```

```@{Hosts.@{HOST_STAGE}}/foo``` would be resolved as follows:
1. `@{Hosts.@{HOST_STAGE}}/foo` `[@{HOST_STAGE} -> "Beta"]` -> `@{Hosts.Beta}/foo`
2. `@{Hosts.Beta}/foo` `[@{Hosts.Beta} -> http://localhost]` -> `http://localhost/foo`




## Interactive Mode

You can execute a test file in interactive mode by specifying your test file with the `-file` parameter along with `-step`.

![Interactive Demo](./.github/images/interactive-demo.gif)



## Run Behavior

All *Test Suites* run in parallel with each other. The tests within each suite will run sequentially to force a linear dependency graph on storing and fetching variables in the data store.
For most optimal performance, you can organize your tests in one of the following ways:
1. `${test-root}/${api}.yaml` - Good for short API calls that all flow into each other.
2. `${test-root}/${api}/{action}.yaml` - Good for separating tests with dependent calls from other tests with no dependencies within the same API scope

## Pro-Tips:

### Using Anchors
Our input is YAML and YAML supports anchors out of the box to reduce verbosity! Since the tests within yaml file are scoped under the 'tests' key, you can create arbitrary keys for anchoring
elsewhere in your test file.

```yaml
# foo_test.yaml

# common headers to pass with every request
Headers: &headers
  SomeHeader: MyHeader

# some common regular expression
# ISO date field
DateField: &date 
  type: string
  matches: '^(\d{4})-0?(\d+)-0?(\d+)[T ]0?(\d+):0?(\d+):0?(\d+)$' 

AnyString: &any-string
  type: string
  matches: $any

# define some constant values here too
Response200: &succes 200

Input: &default-input
  apiToken: @{API_KEY}

# tests is the only reserved key in our file
tests:  
  - name: List Foos
    description: Should list all Foos with a Name ending with 'Bar'
    method: GET
    route: 'http://localhost/foo?search=Bar'
    headers:
      <<: *headers
      NewHeader: something not common
    input:
      <<: *default-input
    response:
      code: *succes
      # expecting a payload of {Foo: [{Date: "FooBar"}, "any string"]}
      payload:
        Foo:
          type: array
          length: 1
          items:
            - type: object
              properties:
                Date: *date
            - *any-string
```

