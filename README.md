# integration-checker
Tool for making JSON API calls that can be used for integration tests, simulation, checks, etc.

## About
This tool provides a lightweight black-box testing framework for your API. 
All you need to provide are your inputs to the route and a validation definition for the response. 
The tool will then execute the requests and run your validation against the response and print a pretty report at the end. All tests are
defined in YAML files, no coding is required.

### Features
* **Test definitions as Config:** Use yaml to define your tests. It's easy to write and less verbose than JSON. You can use anchoring to reduce the amount of copy and paste required
* **Data Persistence:** You can store results that your validation matches and use them as inputs in subsequent tests

### Definitions
* **Test:** An individual test case that provides an input and validates a response
* **Test Suite:** A file containing multiple tests

## Usage
```text
Usage of ./checker:
  -colors
        Whether to print test report with colors (default true)
  -fixtures string
        Path to yaml file with data to include into the test scope via test variables. (default "./fixtures.yaml")
  -host string
        Default host url to use with tests. Populates the @{host} variable. (default "http://localhost")
  -short
        Whether or not to print out a short or extended report (default true)
  -test-root string
        File path to scan and execute test files from (default ".")
  -threads int
        Number of test files to execute at a time. (default 16)
```


## Sample Test
Here is a simple test case that can be executed

```yaml
# foo_test.yaml

# any file with a 'tests' key at the top most level will be executed
tests:
  # you can have multiple tests in the same file
  - name: List Foos
    description: Should list all Foos with a Name ending with 'Bar'
    method: GET
    route: 'http://localhost/foo?search=Bar'
    response:
      code: 200
      # expecting a payload of {Foo: [{Name: "FooBar"}]}
      payload:
        Foo:
          type: array
          length: 1
          items:
            - type: object
              properties:
                Name:
                  type: string
                  matches: .*Bar
```

Executing the test can be done like so:
```./checker -test-root="<path to directory containing foo_test.yaml>"```

Sample output:
```text
[Passed] <test-root>/foo_test.yaml
 Passed: 1, Failed: 0, Total:1
--------------------------------------------------------------------------------
 [Passed]: List Foos - Should list all Foos with a Name ending with 'Bar' -> [GET] http://localhost/foo?search=Bar
  [*] response.StatusCode: 200
  [*] .Foo: [length] 1
  [*] .Foo[0].Name: FooBar

--------------------------------------------------------------------------------
  [Passed] <test-root>
  1     :Total Tests
  1     :Passed
  0     :Failed
--------------------------------------------------------------------------------
```

## Validations

Each JSON data type has its own set of validation rules that can be applied.

### Numbers
```yaml
payload:
  MyNumber:
    type: integer # todo: need to update this to just 'number'
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

In the event that you are validating a large array response and you are looking to seek out a specific element from it. You can use the `sorted` property to have the validation
perform a depth first search for a node matching the validation. E.g:

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

This mechanism can be used to 'pick' values from the response to put into the data store. However, you need to ensure the most specific validation is executed first to locate the correct node. Otherwise
you may get an unexpected node that matches the more general result.

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

Each *Test Suite* has it's own data store that the tests can read and write variables to. Variables are read using `@{myVarName}` notation, and are
written using the `storeAs` property on your field validator.  

Variables can only be read in the following test fields:
* input
* headers
* route



For example, if we wanted to store the ID  of FooBar from the sample test to use in a subsequent GET call. Our test would look like:

```yaml
# foo_test.yaml
tests:
  - name: List Foos
    description: Should list all Foos with a Name ending with 'Bar'
    method: GET
    route: 'http://localhost/foo?search=Bar'
    response:
      code: 200
      # expecting a payload of {Foo: [{Name: "FooBar", Id:"123456789"}]}
      payload:
        Foo:
          type: array
          length: 1
          items:
            - type: object
              properties:
                Name:
                  type: string
                  matches: .*Bar
                Id:
                  type: string
                  matches: [0-9]+
                  # ID for FooBar is now available as 'test_id'
                  storeAs: test_id

  - name: Get FooBar
    description: Should validate that FooBar has information
    method: GET
    # read back the variable using '@{}' notation
    route: 'http://localhost/foo/@{test_id}'
    # response should exists
    response:
      code: 200
```

### Fixtures

The data store can be pre-populated with read only data prior to executing your tests with a file containing data definitions. This file should consist of a map(s) that terminate to
a string. E.g.

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

The data store will also be pre-populated with your systems environment variables and can be access the same way as any other variable

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


### Variable Composition
Variables can also be composed of other variables. For example, lets say we want the host to be determined dynamically based on our systems environment variable:

```shell
#Env Var
export HOST_STAGE="Beta"
```

```yaml
# fixtures.yaml
Hosts:
  Beta: http://localhost
  Prod: http://NotLocalHost.com

# foo_test.yaml
tests:
  - name: List Foos
    # This will be expanded at run time
    # First @{HOST_STAGE} will be resolved to "Beta" -> @{Hosts.Beta}
    # Then @{Hosts.Beta} resolves to 'http://localhost'
    route: '@{Hosts.@{HOST_STAGE}}/foo
...
```

## Run Behavior

All *Test Suites* run in parallel with each other. The tests within each suite will run sequentially to force a linear dependency graph on storing and fetching variables in the data store.
For most optimal performance, you can organize your tests in one of the following ways:
1. ${test-root}/${api}.yaml - Good for short API calls that all flow into each other.
2. ${test-root}/${api}/{action}.yaml - Good for separating tests with dependent calls from other tests with no dependencies within the same API scope

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

