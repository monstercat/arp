# Arp
Short for Arpeggio, is a tool for automating and validating RESTful, RPC, and Websocket network calls. This tool is intended to be used for writing code free, quick, and flexible, integration or end-to-end tests. 

Excluding the validation aspect, it can be used to orchestrate a sequence of network calls where the outputs of a call can be mapped to inputs of the subsequent calls.

## Hilights
* Tests are defined as YAML configuration files, no coding required
* Validations are defined based on the expectation that an API call returns JSON formatted data. Each field can be validated using exact or partial matching.
* Non-JSON formatted responses (e.g. binaries from download URLS, websocket requests) are automatically transformed into a JSON representable object that exposes simple properties for validation (size in bytes, sha256sum). If these properties are not enough, the response can be dumped to a local file and passed to an external program for validation as part of the test definition!
* Individual fields from a test case's JSON response can be persisted and used as inputs or within the field validators for subsequent test cases via variables
* Features an interactive mode for stepping through your test cases, retrying individual tests, viewing stored variables, and even hot-reloading of your test definition


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

## Installation
```shell
export GO111MODULE=on 

git clone https://github.com/monstercat/arp.git && \
  cd arp && \
  go install ./cmd/arp
```

## Usage
```text
Usage of ./arp:
  -always-headers
        Always print the request and response headers in long test report output whether any matchers are defined for them or not.
  -colors
        Print test report with colors. (default true)
  -error-report
        Generate a test report that only contain failing test results.
  -file string
        Path to an individual test file to execute.
  -fixtures string
        Path to yaml file with data to include into the test scope via test variables. This file is also merged with each test file such that any YAML anchors defined within it are available for reference in the test files.
  -short
        Print a short report for executed tests containing only the validation results. (default true)
  -short-fail
        Keep the report short when errors are encountered rather than expanding with details.
  -step
        Run tests in interactive mode. Requires a test file to be provided with '-file'
  -tag value
        Only execute tests with tags matching this value. Tag input supports comma separated values which will execute tests that contain any on of those values. Subsequent tag parameters will AND with previous tag inputs to determine what tests will be run. Specifying no tag parameters will execute all tests.
  -test-root string
        Folder path containing all the test files to execute.
  -threads int
        Max number of test files to execute concurrently. (default 16)
  -tiny
        Print an even tinier report output than what the short flag provides. Only prints test status, name, and description. Failed tests will still be expanded.
  -var value
        Prepopulate the tests data store with a single KEY=VALUE pair. Multiple -var parameters can be provided for additional key/value pairs.
```

TLDR;

All tests in a directory can be executed with the `-test-root` flag like so:

```./arp -test-root="<path to directory containing foo_test.yaml>"```

Individual files can be executed using the `-file` flag:

```./arp -file=<path>/foo_test.yaml```

## Sample Tests



### Natural JSON Response
Here is a simple test case for a REST API returning a JSON response that can be executed:

![REST Output](./.github/images/demo.gif)

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




### Binary Response

When you get a non-JSON response from a network call, it gets transformed into a JSON representable object for validation:

![Binary Output](./.github/images/download.gif)


```yaml
tests:
  - name: Test Download
    description: Make sure we download the expected file
    route: https://www.dundeecity.gov.uk/sites/default/files/publications/civic_renewal_forms.zip
    method: GET
    response:
      code: 200
      # Set the response type to binary so the binary payload can be formatted correctly for easy validation.
      type: binary
      # response will be saved to this file
      filePath: /tmp/myfile.zip
      payload:
        # We can validate the size in bytes if it is known ahead of time
        size:
          type: integer
          matches: $any
        # and/or we can validate the sha256 sum of the data
        sha256sum:
          type: string
          matches: $any
        # This field is always made available (whether a file path is 
        # provided or not) allowing an 'external' validator to process the file.
        saved:
          type: string
          matches: /tmp/myfile.zip
```

---

### Test Structure Breakdown

This is a general overview of the test file structure. Individual examples with further details are also provided in their relevant doc sections. 

```yaml
# test_suite.yaml

# tests is an array of test case objects
tests:
    # name of the test
  - name: <string> 

    # what the test does
    description: <string> 
    
    # If set to true, this test will be skipped
    skip: <bool>

    # Used for both http calls and websocket connections
    route: <string> (<protocol>://<host>[:port]/<path>[?<params>&...])

    # For REST API calls only
    method: 'GET' | 'POST'

    # Headers to send with the request. These are sent on every REST API request and with the first Websocket client
    # connection.
    headers: # <object>
      # Each key refers to the specific header (e.g. Content-Type)
      <string>: <string>

    # A list of string identifiers to associate with the test. This can be used to filter what tests to execute at runtime with the `-tag` parameter
    tags:
      - <string>

    # Root object containing input to send with the request. 
    # This will change depending on the existence of any input modifiers (form_input, websockets, etc.)
    input:
      <string>: <object>|<array>|<bool>|<number>|<string>|<web socket messages>|<form inputs>
      
    # Protocol Modifier/Input modifier
    # If set to true, the contents of `input` will be sent as an HTML form with support for multipart upload.
    # See section 'API Inputs > Multipart/form-data' below for further details
    formInput: <boolean> 
    
    # Protocol/Input Modifier
    # If set to true, the test will spin up a websocket client and connect to the destination provided in `route`.
    #
    # A websocket test can send one or more payloads to fulfill an arbitary request transaction.
    #
    # The test will look for a specifically formatted `input` object based on the configuration below. See 
    # section 'API Inputs > Websocket' below for further details.
    websocket: <bool>
  
    # Protocol Modifier
    # If the following configs are provided, the test will spin up an RPC client and attempt to make a call
    # to the given address.
    rpc:
      protocol: HTTP | TCP
      address: <string>
      procedure: <string>

    # Root object containing instructions on how to validate the call response
    response:
      # Expected status code for an HTTP response. Not available for Websocket or RPC calls
      code: <integer>|<Integer Matcher>

      # Whether or not a binary response is expected. This will format any binary response into a basic representation
      # in JSON that validation matchers can be applied to. This object representation includes things like size in bytes and 
      # sha256 sum of the data
      # Only available for HTTP and RPC response validation
      type: binary | json

      # File path to save any binary response data to. This can be used in conjunction with form uploads to test 
      # downloading and uploading of files
      filePath: <string>

      # Expected response headers to create matchers for. See the `Validations> Response Headers` section for more details.
      headers:
        <header name>: <Array Matcher>
        
      # Expected response matchers. Arp will always generate a response represented in JSON format that matchers can be
      # created for. This JSON representation may change depending on the nature of the response. See the `Validations` 
      # section for information on writing validators.
      payload:
        <string>: <Any Matcher>
```

## API Inputs

There are currently 3 supported ways to provide inputs to your API request:
* query parameters (REST)
* JSON input (REST + RPC)
* Multipart/form-data (REST: fields + multipart uploads + multi-file uploading)
* Websockets (text and binary)

### Query Parameters
Query parameters are simply added to the `route` property of your test case.

```yaml
# sample.yaml
tests:
  - name: "List Users"
    description: "Listing all users on page 2"
    method: "GET"
    # add your parameters like you usually would
    route: "https://reqres.in/api/users?page=2"
---
```

### JSON Input (REST/RPC)
JSON input can be provided using the `input` property of your test case for `POST` or any `RPC` request.

```yaml
# create.yaml
tests:
  - name: "Create User"
    description: "Create a test user"
    method: "POST"
    route: "https://reqres.in/api/users"

    # pass in any json formatted input here
    input:
      name: "test user"
      job: "tester"
  
    response:
      code: 201
      payload:
        createdAt:
          type: string
          matches: $any
        id:
          type: string
          matches: $any
```

The input sent over the wire looks like:

```json
{"name":  "test user", "job":  "tester"}
```

### Multipart/form-data
You can specify that your input should be submitted as an HTML form by setting `formInput: true` in your test case. This mechanism can be used to upload one or more files.
Form field names are defined by their key in the `input` property and are populated with the values they are mapped to. Entries that map to an array are treated as file form fields where each array element should be a file path that is to be uploaded with the form.

The request's Content-Type header will automatically be set for multipart forms.

For example, the following form
```html
<form action="localhost/send" method="post" enctype="multipart/form-data">
    <input type="file" name="file" required="">
    <input type="text" name="description" value="" required="">
</form>
```

Can be submitted like so with the test
```yaml
tests:
  - name: Test form submissions
    description: Upload a file through a form
    method: POST
    route: <some file uploader site>
    
    # set our input as form data
    formInput: true
    input:
      # populates a multipart form field called "files" in which one or more files will be uploaded
      file: 
        - /tmp/random.txt

      # populuates a multipart form field called "description" with the value "text file"
      description: "text file"
  
    response:
      code: 200
```


### Websocket

You can set your test to make a websocket call by setting `websocket: true` in your test case. The first test case to have this flag enabled will be the one that creates
the websocket client to the `route` specified in the test case. This client is then re-used for all test cases in the test suite that have `websocket: true` until the client is closed (input.closed: true).

The input object should be formatted as follows:

```yaml
tests:
  ...
  
  websocket: true
  input:
    # array of websocket messages
    requests: 
        # <Websocket Message>
        # What message type to send the websocket request. Default: text
      - type: binary | text

        # How the payload is encoded when sending 'binary' websocket messages. Default: base64gzip
        #
        # * Base64 encoded embedded GZIP: your contents are gzipped and encoded as base64 explicitly for embedding in
        #        this test file. The test client will decode and extract the data to send the raw bytes over the wire
        # * Hex: you have an explicit set of bytes that you want to send. The test client will decode hex input and
        #        send the raw bytes over the wire.
        # * File: you have a local file that you want to send. The test client will read the entire file to memory and
        #        send the raw bytes over the wire.
        # * External: you want to call an external program/binary that will generate or encode your requests input at runtime.
        #           The standard output of the specified program will be streamed through the established websocket connection. If
        #           a non-zero exit code is returned, the test will fail and the executables STDERR contents will be provided
        #            
        encoding: base64gzip | hex | file | external

        # Payload to send in the request.
        payload: <object> | <string> | <external binary path>

        # Pass the following arguments to the binary specified in the `payload` when `encoding` is set to `external.
        args:
          - <string>
          - <string>
        
        # If set to true, the test client will not wait for a response and continue to send the next websocket message.
        # Default: false
        # Please note that setting `writeOnly: true` and making a call that DOES illicit a response will break 
        # the order in which test validations are run and likely fail your test.
        writeOnly: <bool>

        # If set to true, no message will be sent and the client will only wait for a response. 
        # Default: false
        # Having this and writeOnly enabled will create a no-op for a given message. 
        readOnly: <bool>

        # Use the writeOnly and readOnly flags to setup the ordering of who is expected to write or read
        # first when the websocket connection is created.
        
        # How the response data should be handled.
        # json: response data will attempted to be unmarshalled into a JSON object that can be validated.
        #       It will fallback to the binary json representation if the unmarshalling fails.
        # text: response data will be treated as plain text and wrapped in a json object for validation.
        #       This json object format follows { "payload": "<text response>" }.
        # binary: response data will be handled as a stream of data and the standard binary JSON response 
        #         will be provided for validation. This is useful for large responses that may not fit
        #         in memory.
        response: json | text | binary

        # Save the output of a binary response to a specific file path. This can then be passed into an
        # external validator to validate the binary contents.
        filePath: <string>
      - ...
      
    # If false, the next websocket enabled test will re-use the client from the last non-closed websocket test.
    # Set this to true if you want to force a new websocket session for the following test case
    close: <bool>
```

Multiple websocket writes can be performed within a single test case:

```yaml
tests:
  - name: Testing websockets!
    description: Gonna make a websocket call
    route: ws://localhost:8080/echo
    websocket: true
    input:
      requests: 
          # first request sends a JSON object
        - type: text # set message type to text
          payload: 
            data: gonna send a json object
          response: json
          
          # second sends just plain text
        - type: text
          payload: just text
          response: text
          
          # Third sends plain text but expects binary data in the response
        - type: text
          payload: gimme a binary
          response: binary

          # fourth sends a binary message and expects a text response
        - type: binary
          # Set our payload encoding as hex for sake of embedding it in the test file.
          # The test client will decode the hex input and send the raw byte representation over the wire
          encoding: hex
          payload: 68656c6c6f2c2068657820776f726c64
          response: text

          # Using the read and write only flags to make this message a no-op
        - readOnly: true
          writeOnly: true
          payload: This is a no-op and nothing will be done

          # Send the standard output of a program as our websocket payload
        - type: text
          encoding: external
          payload: /bin/date
          args:
            - '-u'
            - '-R'
```

## Validations

Each JSON data type has its own set of validation rules that can be applied. Some types have a short form available that support a more limited feature set of the regular validation definition. All short forms (other than strings) do not support variables from data store since the matcher type is derived from the value specified prior to the test execution.

### Integers
```yaml
payload:
  MyInteger:
    type: integer
    exists: <bool> # defaults to true
    matches: <matcher>
```

Supported matchers:
* a specific integer value: e.g. -1, 0, 1, 2, 3, ...
* The **$any** key word to match regardless of the numerical value

#### Short form
Only supports integer constant values.

```yaml
payload:
  MyInteger: 42

# Is the same as
payload:
  MyInteger:
    type: integer
    exists: true
    matches: 42
```

### Numbers
```yaml
payload:
  MyNumber:
    type: number
    exists: <bool> # defaults to true
    matches: <matcher>
```

Supported matchers:
* a specific numerical value: e.g. 1, 2, 3, 3.14, etc.
* The **$any** key word to match regardless of the numerical value

#### Short form
Only supports numerical constant values.

```yaml
payload:
  MyNumber: 42.1

# Is the same as
payload:
  MyNumber:
    type: number
    exists: true
    matches: 42.1 
```
### Booleans
```yaml
payload:
  MyBool:
    type: bool
    exists: <bool> # defaults to true
    matches: <matcher>
```

Supported matchers:
* true or false
* The **$any** keyword to match regardless of the numerical value

#### Short form
Only supports boolean constant values.

```yaml
payload:
  MyBool: true

# Is the same as
payload:
  MyBool:
    type: bool
    exists: true
    matches: true
```


### Strings
```yaml
payload:
  MyString:
    type: string
    exists: <bool> # defaults to true
    matches: <matcher>
```

Supported matchers:
* any regular expression: e.g. ".*", "SomePartial.+", "[0-9]+"
* The **$any** keyword to match any string (".*" expression)
* The **$notEmpty** keyword to match non-empty strings (".+" expression)

#### Short form
Supports all string matchers.

```yaml
payload:
  MyString: "meaning of life"

# Is the same as
payload:
  MyString:
    type: string
    exists: true
    matches: "meaning of life"

# Also supported
payload:
  MyString: <$any>|<$notEmpty>|<regexp>
```

### Arrays
```yaml
payload:
  MyArray:
    type: array
    length: <matcher>
    sorted: <bool> # defaults to true
    exists: <bool> # defaults to true
    items:
      - <sub validations>
```

Supported matchers (for length):
* any integer value: e.g. 1, 2, 200, ...
* The **$notEmpty** keyword
* Length expressions like: **$< 5**, **$<= 1**, **$> 1**, **$>= 500**

#### Short form
Length matcher is fixed to `$notEmpty`.

```yaml
payload:
  MyArray:
    - "test String"
    - type: object
      properties:
        Email: ".*@.*"

# Is the same as
payload:
  MyArray:
    type: array
    length: $notEmpty
    exists: true
    sorted: true
    items:
      - type: string
        matches: "test sttring"
      - type: object
        properties:
          Email:
            type: string
            matches: ".*@.*"
```


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

#### Sorted Arrays
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

#### Short form
No Short form is available for Object validations at this point in time. There is no way to determine whether an object will be a test definition or is part of the expected payload due to potential collisions with the `type` and `property` field on valid response objects.


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


### Response Code

Validating API response codes in the `payload` uses any valid integer validator.

```yaml
# match 200 exactly
payload:
  code: 200

# match any non-500
payload:
  code:
    type: integer
    matches: '[1234][0-9]{2}'

# match any code
payload:
  code:
    type: integer
    matches: $any
```

### Response Headers
 You can define validations for response headers by defining your validators on the `headers` object of the `response` section in the test. All headers follow the format of `Map[header key] -> []string`

```yaml
tests:
  - name: Get Charles ID
    description: Get the ID for user charles
    route:  https://reqres.in/api/users
    method: GET
    response:
      code: 200
      # ----------------------------------------------------------------
      # Make sure we're getting the right content type in our response
      headers:
        Content-Type:
          type: array
          length: $notEmpty
          items:
            - type: string
              matches: "application/json"
      # ----------------------------------------------------------------
      payload:
        data:
          type: array
          length: $notEmpty
          sorted: false
          items:
            - type: object
              properties:
                email:
                  priority: 0
                  type: string
                  matches: &charles_email 'charles.morris@reqres.in'
                id:
                  priority: 1
                  type: integer
                  matches: $any
                  storeAs: charles_id    
```

You can clean up your header validators using the supported short forms for strings and arrays like so:

```yaml
---
response:
  headers:
    Content-Type:
      - "application/json'
---
```

### Binary Response Validation

You can write (limited) tests to validate binary specific response data. This is done by specifying `binary:true` in the `response` section of the test. The sha256 sum of the response data and its size in bytes are made available to matchers. Furthermore, the response can can be saved to a specific path on disk using the 'filePath' parameter which can then subsequently be used for future upload calls or external validation.

```yaml
tests:
  - name: Test Download
    description: Make sure we download the expected file
    route: https://www.dundeecity.gov.uk/sites/default/files/publications/civic_renewal_forms.zip
    method: GET
    response:
      code: 200
      # set the response type to binary so the binary payload can be formatted correctly for easy validation
      type: binary
      # response will be saved to this file
      filePath: /tmp/myfile.zip
      payload:
        # We can validate the size in bytes if it is known ahead of time
        size:
          type: integer
          matches: $any
        # and/or we can validate the sha256 sum of the data
        sha256sum:
          type: string
          matches: $any
        # This field is always made available in the event that the response is stored into a temp file
        # for tests expecting a JSON response but end up with a binary response.
        saved:
          type: string
          matches: /tmp/myfile.zip
```

If a call is made where non-binary data is expected but the response *does* contain binary data, the response will automatically fallback to the binary response format with some messages indicating the fallback was made. Your test will only fail if you had any validators defined on specific fields of the payload, otherwise you can continue to validate only the status code if you don't really care about the response.

```json
  response: {
   "NOTICE": [
    "Unexpected non-JSON response was returned from this call triggering a fallback to its binary representation.",
    "Response data has been written to the path in the 'saved' field of this object."
   ],
   "saved": "/var/folders/47/56666ndd29n748s01t3f4cdr0000gn/T/binary-response-483156933",
   "sha256sum": "c8bceaae2017481d3e1fd5b47fc67d93f1e049c057461effabb9152b571d65b2",
   "size": 6615
  }
```

### Websocket Response Validation

You can write tests to validate your websocket responses similar to how regular JSON and binary responses are validated. Since multiple writes/reads can happen in a given websocket test
case, the response payload will contain array that correlates back to each websocket message object provided as an input.

```yaml
# sample.yaml
# Running against the gorilla sample websocket server
# https://github.com/gorilla/websocket/blob/master/examples/echo/server.go
tests:
  - name: Testing websockets!
    description: Gonna make a websocket call
    route: ws://localhost:8080/echo
    websocket: true
    input:
      requests: 
          # first request sends a JSON object
        - type: text # set message type to text
          payload: 
            data: gonna send a json object
          response: json
          
          # second sends just plain text
        - type: text
          payload: just text
          response: text
          
          # Third sends plain text but expects binary data in the response
        - type: text
          payload: gimme a binary
          response: binary

          # fourth sends a binary message and expects a text response
        - type: binary
          # Set our payload encoding as hex for sake of embedding it in the test file.
          # The test client will decode the hex input and send the raw byte representation over the wire
          encoding: hex
          payload: 68656c6c6f2c2068657820776f726c64
          response: text
    response:
      payload:
        # Root object for all websocket responses
        responses:
          # validate the response of the first websocket text request
          - type: object
            properties:
              data: "gonna send a json object"
              
          # validate the response of the second websocket text request
          # even though we sent plain text and the server responded with plain text, it was wrapped 
          # in an object for validation purposes:
          # we sent: "just text" -> server echos "just text" -> test client wraps response {"payload": "just text"} -> validation
          - type: object
            properties:
              payload: "just text"
              
          # validate the binary response of the third websocket text request
          - type: object
            properties:
              size:
                type: integer
                matches: $any
              sha256sum: $any
          
          # the response from our binary sent data is just plain text
          - type: object
            properties:
              payload: 'hello, hex world'
```

#### Websocket Sessions

By default, a websocket connection will remain open in between test cases to preserve the same session for follow-up transactions. However, you can tell the test close the client to initiate a new session in a follow-up test by setting 
`close: true` in the test input. 

The test client will automatically close after all tests in a test suite have been executed - no need to explicitly close the client on your last test.


Here's an example of session closing/sharing:

```yaml
tests:
  - name: Testing websockets!
    description: Gonna make a websocket call
    route: ws://localhost:8080/echo
    websocket: true
    input:
      requests: 
        - payload: 
            object: gonna send a json object
          response: json
        - payload: just text
          response: text
        - payload: gimme a binary
          response: binary
          
      # Close the websocket client after this tests transactions have completed
      close: true          
    response:
      payload:
        responses:
          - type: object
            properties:
              object: "gonna send a json object"
          - type: object
            properties:
              payload: "just text"
          - type: object
            properties:
              size:
                type: integer
                matches: $any
              sha256sum: $any


  - name: Re-open the client
    description: New call with new client
    route: ws://localhost:8080/echo
    websocket: true
    input:
      requests: 
        - payload: 
            object: new client, new text
          response: json    
      # Close is not specified (defaults to false). The client created by THIS test will be used in the next
      # websocket test case.
    response:
      payload:
        responses:
          - type: object
            properties:
              object: $any
      
      
  - name: Re-use open client
    description: New call with old client
    route: ws://localhost:8080/echo
    websocket: true
    input:
      requests: 
        - payload: 
            object: old client, new text
          response: json     
      # Closing the websocket client connection created from the previous test. The next websocket test will create
      # a new client connection.
      close: true
    response:
      payload:
        responses:
          - type: object
            properties:
              object: $any
```

### External Validator

You can specify an external validator if none of the above built in ones are sufficient for your test case. This is done by passing the response value to an external executable as a program parameter and validating the status code of the execution.  

The main limitation is that all response values that are to be passed into external executables must be representable as strings. For binary responses, this can be achieved by using the built in mechanisms to save the binary data to a file and pass that file path into to the external validator.

#### Binary Data

Here is a modified form of the file download example that passes in the downloaded file to an external executable:
```yaml
tests:
  - name: Test Download
    description: Make sure we download the expected file
    route: https://www.dundeecity.gov.uk/sites/default/files/publications/civic_renewal_forms.zip
    method: GET
    response:
      code: 200
      # set the response type to binary so the binary payload can be formatted correctly for easy validation
      type: binary
      # response will be saved to this file
      filePath: /tmp/myfile.zip
      payload:
        # We can validate the size in bytes if it is known ahead of time
        size:
          type: integer
          matches: $any
        # and/or we can validate the sha256 sum of the data
        sha256sum:
          type: string
          matches: $any

        # This field stores the file path of our downloaded file, we can add the
        # external validator here!
        saved:
          type: external
          # what exit code we expect the program to return. The exit code will not be validated if this field is missing.
          returns: 0
          # we want to execute a python script that will do something with the
          # zip file and return a 0 exit code on success.
          bin: /usr/local/bin/python3
          args:
            - '@{TEST_DIR}/test.py'
            - /tmp/myfile.zip
```

Similarily with websockets:

```yaml
tests:
  - name: Testing websockets!
    description: Gonna make a websocket call
    route: ws://localhost:8080/echo
    websocket: true
    input:
      requests: 
        - type: binary
          encoding: hex
          payload: 68656c6c6f2c2068657820776f726c64
          response: binary
          filePath: &ws_resp1 '/tmp/websocket_response_1'
    response:
      payload:
        responses:
          - type: object
            properties:
              saved:
                type: external
                bin: /usr/local/bin/python3
                args:
                  - '@{TEST_DIR}/test.py'
                  - *ws_resp1
```

#### Non-Binary Resopnse Data
For non-binary data that isn't being written to a file, there is no pre-determined string (like a filepath) that can be used to pass the value in as an argument to the external program. This can be solved with the `storeAs` field of the validator to set a data store variable with the value that can then be referenced in the arguments array.

Mirroring the above websocket test, lets modifiy it so it doesn't need to write any file. Suppose we call the server endpoint that echos everything we send back as base64, and now
we have a script to decode the response and perform an equality check:

```bash
#!/usr/bin/env bash
# /tmp/checkBase64.sh
# Decodes the first base64 input and compares it with the second plain text input.

input="$1"
expected="$2"

result=$(echo "${input}" | base64 -d)

echo "Decoded: ${result}"
echo "Expected: ${expected}"

if [ "${result}" = "${expected}" ]; then
  # all good, return 0!
  exit 0
fi

# not equal, fail the validation!
exit 1
```

Our test case would look like this:
```yaml
tests:
  - name: Base64 Echo Testing
    description: Make a websocket call that echos the input as base64

    # we are now calling an API that will echo everything back in base64
    route: ws://localhost:8080/base64
    websocket: true
    input:
      requests: 
        - type: text
          payload: Hello, world
          response: text
    response:
      payload:
        responses:
          - type: object
            properties:
              payload:
                type: external
                # store the response in the data store
                storeAs: encoded_response
                bin: '@{TEST_DIR}/checkBase64.sh'
                args:                  
                    # Now we can reference the value as a variable for our scripts input
                  - '@{encoded_response}'
                  - 'Hello, world'
```



## Test Tags

Each test can have an array of arbitrary tags defined that can then be used filter test execution at runtime. This is useful for creating sets of tests that may be executed in one context but not another. These tags are defined in the `tags` field of the test definition like so:

```yaml
# tests.yaml
tests:
  - name: Get Stuff
    description: This test performs a read only operation
    method: GET
    tags: 
      - read
      # create a local tag to identify a test that will work in a local environment
      - local
    ...

  - name: Write stuff
    description: This test performs a write operation
    method: POST
    tags:
      - write
    ...
```

Now suppose we want to execute our read-only tests:

```bash
# execute tests with tag 'read'
arp -file=./tests.yaml -tag=read
```

If we want to execute both read and write tests, we simply provide the tags as a comma separated list for a single argument. This will perform an OR check on the tags.

```bash
# execute tests with tag 'read' OR 'write' defined
arp -file=./tests.yaml -tag=read,write
```

We can add an additional `-tag` parameter if we want to only execute tests that are known to work in a local environment. This will perform an AND check with all the provided `-tag` inputs:

```bash
# execute tests with tags 'read' AND 'local' defined
arp -file=./tests.yaml -tag=read -tag=local
```

Combining these, we can now execute all read and write tests that work in our local environment:

```bash
# execute tests that have tags 'read' OR 'write', AND 'local' defined
arp -file=./tests.yaml -tag=read,write -tag=local
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

### Variable Syntax

Variables support JSON dot-like syntax for storing and reading from the data store. 
If a write is made to an object or path that doesn't exist in the data store, the required data structures will be automatically created

E.g. 

```yaml
# Storing a value
    id:
      type: integer
      matches: $any
      storeAs: someObj.someArray[3].Id

---
# Using the stored value    
    response:
      payload:
        data:
          type: object
          properties:
            id: '@{someObj.someArray[3].Id}'
```


### Fixtures

The data store can be pre-populated with read only data prior to executing your tests with a file containing data definitions. This file should consist of a maps and arrays that terminate to a string value. E.g.

```yaml
#fixtures.yaml
Hosts:
  Beta: 
    Local: http://localhost
    RealBeta: http://beta.NotLocalHost.com
  Prod: http://NotLocalHost.com
Search:
  - Bar
  - Bazz

# foo_test.yaml
tests:
  - name: List Foos by Bar
    # Access our host using the fixture mapping with dot ('.') notation
    # and index into our array using '[<index>]'
    route: '@{Hosts.Beta.Local}/foo?search=@{Search[0]}'
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

These are added to the data store AFTER the fixtures file has been loaded for each test suite. This allows you to override values in
the fixture file with a runtime value instead.

```yaml
# fixtures.yaml
SomeData:
  WithAnArray:
    - AnotherObj:
        MyValue: Override This

```

```bash
./arp -test-root=. -fixtures=fixtures.yaml -var='SomeData.WithAnArray[0].AnotherObj.MyValue=Overwritten!'
```



If the target JSON path doesn't exist, empty structures leading to the value will be automatically created:
```yaml
SomeData:
  DontUseThis:
    - Nothing
```

```bash
./arp -test-root=. -var='SomeData.SomeMapping.UseThisInstead[5].Value=Hello'
```

results in:
```
@{SomeData} -> {
 "DontUseThis": [
  "Nothing"
 ],
 "SomeMapping": {
  "UseThisInstead": [
   null,
   null,
   null,
   null,
   null,
   {
    "Value": "Hello"
   }
  ]
 }
}
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


## Dynamic Inputs

The `input` properties of Test Cases also have the ability to use the output of an executed command as its value. This works similarly to the behavior of variables where it'll perform a straight value replacement (and recursive execution), but uses the syntax of ```$(<path> arg1 arg2 ...)```.
The output is treated as a regular string and contains both the STDOUT and STDERR data. If the program encounters an error (program returns a non-zero exit code), the test case will not be executed. 

For example, you can use `/bin/date` to provide the current date with your API call:

```yaml
  input:
    date: '$(/bin/date -u -R)'
```

This syntax also supports the usage of variables. All variables are resolved prior to the execution of the command.

```yaml
  input:
    something: '$(@{TEST_DIR}/myscript.sh)'
```

Or you can run some arbitrary shell command:

```yaml
  input:
    result: '$(/bin/bash -c "echo \"hello, world\" | base64")'
```

Arp will recursively execute programs provided in the input starting with the inner most `$(<command>)` allowing for execution chaining like the following:

```yaml
  input:
    result: '$(/bin/echo $(/bin/echo "first") $(/bin/echo "second"))'
```

The above example will execute in the following order:
1. `$(/bin/echo $(/bin/echo "first") $(/bin/echo "second")) -> [$(/bin/echo "first") -> first] -> $(/bin/echo first $(/bin/echo "second"))`
2. `$(/bin/echo first $(/bin/echo "second")) -> [$(/bin/echo "second") -> second] -> $(/bin/echo first second)`
3. `$(/bin/echo first second) -> "first second"`

This unfortunately means that you won't be able to use bash subshells if you're looking to straight up embed a complicated bash command. This recursive execution strategy will allow for future expansion with built-in methods 
that don't exist outside of arp. In such a scenario it's recommended to put your command in an external script and 
invoke that script as a dynamic input.

### Using '|' and '>'
YAML has built in operators to allow strings that span multiple lines. These operators can be used to improve readability of longer embedded scripts.
> Values can span multiple lines using | or >. Spanning multiple lines using a “Literal Block Scalar” | will include the newlines and any trailing spaces. Using a “Folded Block Scalar” > will fold newlines to spaces; it’s used to make what would otherwise be a very long line easier to read and edit. In either case the indentation will be ignored. - https://docs.ansible.com/ansible/latest/reference_appendices/YAMLSyntax.html

```yaml
    input:
      # Using '|' will preserve the spacing
      someString: |
        $(/bin/bash -c `
          export SOMEVAR="yay"
          echo '$SOMEVAR'
          echo "$SOMEVAR"
          echo "What have I done?"

          if [ "$SOMEVAR" = "yay" ]; then
            echo "IF WAS TRUE!"
          fi
        `)
```

generates the following input:
```json
  Input: { 
   "someStrg": "$SOMEVAR\nyay\nWhat have I done?\nIF WAS TRUE!\n"
  }
...
```

---

This type of dynamic input is not recommended for providing large amounts of data as it will load the entire result in memory. For multi-part form and websockets requests, it's recommended to use their native binary or file
operations that will allow for data streaming.


## Interactive Mode

You can execute a test file in interactive mode by specifying your test file with the `-file` parameter along with `-step`.

![Interactive Demo](./.github/images/interactive-demo.gif)



## Run Behavior

All *Test Suites* run in parallel with each other. The tests within each suite will run sequentially to force a linear dependency graph on storing and fetching variables in the data store.
For most optimal performance, you can organize your tests in one of the following ways:
1. `${test-root}/${api}.yaml` - Good for short API calls that all flow into each other.
2. `${test-root}/${api}/{action}.yaml` - Good for separating tests with dependent calls from other tests with no dependencies within the same API scope

## Pro-Tips:

### Input Warnings

Data provided in a test case's input is resolved at test execution time and NOT while the config is parsed. 
Make sure to watch out for typos when dealing with websockets that use a mostly fixed form input.


### Using Anchors
Our input is YAML and YAML supports anchors out of the box to reduce verbosity! Since the tests within yaml file are scoped under the 'tests' key, you can create arbitrary keys for anchoring elsewhere in your test file.

This can be useful for creating your own 'short forms' for types without sacrificing the configurability on them.

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

AnyInteger: &not500
  type: integer
  matches: '[234][0-9]{2}'


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
      code: *not500
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

The fixture and test files are concatenated together on load making anchors within the fixtures file available for reference in your test cases as well. This can be used to provide common anchors that can be used across all test files without having to redefine them in each test file.