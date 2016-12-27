// BEGIN: Script Manifest

var MANIFEST_SCRIPT_ID = "cbc-home-page";
var MANIFEST_SCRIPT_NAME = "CBC.ca Home Page";
var MANIFEST_SCRIPT_DESC = "Tests navigation from the CBC.ca home page to the sports page";

// END: Script Manifest

// BEGIN: Target settings
var TargetUrl = "http://www.cbc.ca";
var SportsLinkSelector = "a[href$='sports']"; // =$ means href ends with "sports"
var UserAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) " +
                "Ubuntu Chromium/53.0.2785.143 Chrome/53.0.2785.143 Safari/537.36";
var DefaultPageLoadTimeout = 10000; // 10 seconds
// END: Target settings 

// BEGIN: Screen capture settings
var EnableScreenCapture = true;
var TargetViewports = [
    {name:"desktop", width:1600, height:900},
    {name:"mobile-landscape", width:640, height:360},
    {name:"mobile-portrait", width:360, height:640},
];
// END: Screen capture settings 

// BEGIN: test suite definition 
casper.test.begin(MANIFEST_SCRIPT_DESC, TargetViewports.length * 2, function suite(test) {
    
    // set the user agent to our choice
    casper.userAgent(UserAgent);

    // Retrieve the page from the address specified in TargetUrl
    casper.start(TargetUrl);

    casper.then(function() {

        // Loop through the TargetViewports array, and change the virtual resolution
        casper.each(TargetViewports, function(casper, item) {
            
            casper.then(function() { casper.viewport(item.width,item.height);  });
            // Reload the page, and wait for a second to allow the page to be ingested
            casper.thenOpen(TargetUrl, function() { casper.wait(1000); });

            casper.then(function() {                
            
                // Current viewport is in effect
                test.info("------------------------------------------------");
                test.info("Current viewport: " + item.name + "(" + item.width + "," + item.height + ")");
                test.info("------------------------------------------------");

                // Proceed to test checks and assertions:
                test.assertTitleMatch(/CBC.ca/, 'The CBC.ca home page title matches the regex');
               
                // Since we cannot predict when the page would completely load, a safe approach is to 
                // have Casper wait for a selector to become available inside the DOM
                casper.waitForSelector(SportsLinkSelector, 
                    function selectorFound () { 
                        test.pass("The Sports link was found"); 

                        casper.then(function() {
                            // Take a screenshot of this page                        
                            if (EnableScreenCapture == true) {
                                var screenshotFilename = 'cbc-home-page-' + item.width 
                                    + '-' + item.height + '.png';
                                casper.capture(screenshotFilename, { top: 0, left:0, 
                                    width: item.width, height: item.height });
                            }  
                        });
              
                        casper.then(function() {
                            // Perform a virtual click on the Markets button, 
                            // in effect navigating to that page
                            casper.click(SportsLinkSelector);
                        });

                        casper.then(function() {

                            // Wait until the next page loads, and take a screenshot of that one as well
                            var sportsPageUrlRegex = new RegExp("http://www.cbc.ca/sports");
                            casper.waitForUrl(sportsPageUrlRegex, function() {

                                // Take a screenshot of this page
                                if (EnableScreenCapture == true) {
                                    var screenshotFilename = 'cbc-sports-page-' + item.width 
                                        + '-' + item.height + '.png';
                                    casper.capture(screenshotFilename, { top: 0, left:0, 
                                        width: item.width, height: item.height });
                                }   
                            }, DefaultPageLoadTimeout);   

                        });
                        
                    }, 
                    function failOrTimeout () { test.fail("The Sports link was not found or the page timed out"); }, 
                    DefaultPageLoadTimeout
                );

            }); // END: casper.then(...) per viewport

        });
        // END: casper.each(TargetViewports,...)      
    });    

    // instruct Casper to run the test suite
    casper.run(function() {
        test.done();
    });
});
// END: test suite definition 