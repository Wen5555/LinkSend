#import <Foundation/Foundation.h>
#include <string.h>

static NSMutableDictionary<NSString *, NSMutableDictionary<NSString *, NSURL *> *> *linksendScopedShareURLs;

char *linksendShareContainerPath(void) {
    @autoreleasepool {
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
