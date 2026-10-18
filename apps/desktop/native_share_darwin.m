#import <Foundation/Foundation.h>
#import <Security/Security.h>
#import <Security/SecTask.h>
#include <string.h>

static NSMutableDictionary<NSString *, NSMutableDictionary<NSString *, NSURL *> *> *linksendScopedShareURLs;

static BOOL linksendHasSharedGroupSignature(void) {
    SecCodeRef self = NULL;
    SecStaticCodeRef code = NULL;
    CFDictionaryRef info = NULL;
    BOOL allowed = NO;
    if (SecCodeCopySelf(kSecCSDefaultFlags, &self) == errSecSuccess &&
        SecCodeCopyStaticCode(self, kSecCSDefaultFlags, &code) == errSecSuccess &&
        SecCodeCopySigningInformation(code, kSecCSSigningInformation, &info) == errSecSuccess && info) {
        CFTypeRef team = CFDictionaryGetValue(info, kSecCodeInfoTeamIdentifier);
        if (team && CFGetTypeID(team) == CFStringGetTypeID() && CFStringGetLength((CFStringRef)team) > 0) {
            SecTaskRef task = SecTaskCreateFromSelf(kCFAllocatorDefault);
            if (task) {
                CFTypeRef groups = SecTaskCopyValueForEntitlement(task, CFSTR("com.apple.security.application-groups"), NULL);
                if (groups && CFGetTypeID(groups) == CFArrayGetTypeID()) {
                    allowed = CFArrayContainsValue((CFArrayRef)groups, CFRangeMake(0, CFArrayGetCount((CFArrayRef)groups)), CFSTR("group.com.linksend.desktop"));
                }
                if (groups) CFRelease(groups);
                CFRelease(task);
            }
        }
    }
    if (info) CFRelease(info);
    if (code) CFRelease(code);
    if (self) CFRelease(self);
    return allowed;
}

char *linksendShareContainerPath(void) {
    @autoreleasepool {
        // Ad-hoc preview builds must not enter another app's group container.
        if (!linksendHasSharedGroupSignature()) return NULL;
        NSURL *url = [[NSFileManager defaultManager]
            containerURLForSecurityApplicationGroupIdentifier:@"group.com.linksend.desktop"];
        return url.fileSystemRepresentation ? strdup(url.fileSystemRepresentation) : NULL;
    }
}

char *linksendResolveShareBookmark(const char *requestID, const char *bookmark) {
    if (!requestID || !bookmark) return NULL;
    @autoreleasepool {
        NSString *request = [NSString stringWithUTF8String:requestID];
        NSString *encoded = [NSString stringWithUTF8String:bookmark];
        if (request.length != 32 || encoded.length == 0) return NULL;
        @synchronized([NSFileManager class]) {
            NSURL *cached = linksendScopedShareURLs[request][encoded];
            if (cached) return strdup(cached.fileSystemRepresentation);
        }
        NSData *data = [[[NSData alloc] initWithBase64EncodedString:encoded options:0] autorelease];
        if (!data) return NULL;
        BOOL stale = NO;
        NSError *error = nil;
        NSURL *url = [NSURL URLByResolvingBookmarkData:data
            options:(NSURLBookmarkResolutionWithSecurityScope | NSURLBookmarkResolutionWithoutUI)
            relativeToURL:nil bookmarkDataIsStale:&stale error:&error];
        if (!request || !url || stale || !url.isFileURL || ![url startAccessingSecurityScopedResource]) return NULL;
        @synchronized([NSFileManager class]) {
            if (!linksendScopedShareURLs) linksendScopedShareURLs = [[NSMutableDictionary alloc] init];
            NSMutableDictionary<NSString *, NSURL *> *requestURLs = linksendScopedShareURLs[request];
            if (!requestURLs) {
                requestURLs = [NSMutableDictionary dictionary];
                linksendScopedShareURLs[request] = requestURLs;
            }
            NSURL *existing = requestURLs[encoded];
            if (existing) {
                [url stopAccessingSecurityScopedResource];
                return strdup(existing.fileSystemRepresentation);
            }
            requestURLs[encoded] = url;
        }
        return strdup(url.fileSystemRepresentation);
    }
}

void linksendReleaseShareAccess(void) {
    @synchronized([NSFileManager class]) {
        for (NSDictionary<NSString *, NSURL *> *requestURLs in linksendScopedShareURLs.allValues)
            for (NSURL *url in requestURLs.allValues) [url stopAccessingSecurityScopedResource];
        [linksendScopedShareURLs release];
        linksendScopedShareURLs = nil;
    }
}

void linksendReleaseShareRequest(const char *requestID) {
    if (!requestID) return;
    @autoreleasepool {
        NSString *request = [NSString stringWithUTF8String:requestID];
        @synchronized([NSFileManager class]) {
            NSDictionary<NSString *, NSURL *> *requestURLs = linksendScopedShareURLs[request];
            for (NSURL *url in requestURLs.allValues) [url stopAccessingSecurityScopedResource];
            [linksendScopedShareURLs removeObjectForKey:request];
        }
    }
}
