#import <Cocoa/Cocoa.h>
#import <Security/SecTask.h>
#include <fcntl.h>
#include <limits.h>
#include <string.h>
#include <sys/stat.h>
#include <unistd.h>

extern int linksendReceiveFinderPaths(char *encoded);

int linksendShowNativeEntryFailure(const char *message) {
    if (![NSThread isMainThread]) return 1;
    @autoreleasepool {
        [NSApplication sharedApplication];
        [NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
        [NSApp activateIgnoringOtherApps:YES];
        NSAlert *alert = [[[NSAlert alloc] init] autorelease];
        alert.alertStyle = NSAlertStyleCritical;
        alert.messageText = @"LinkSend";
        alert.informativeText = [NSString stringWithUTF8String:message] ?: @"无法添加所选内容，请打开 LinkSend 后重试。";
        [alert addButtonWithTitle:@"知道了"];
        [alert runModal];
    }
    return 0;
}

char *linksendCanonicalProfileDirectory(const char *path) {
    int fd = open(path, O_EVTONLY | O_CLOEXEC);
    if (fd < 0) return NULL;
    struct stat info;
    char resolved[PATH_MAX];
    BOOL valid = fstat(fd, &info) == 0 && S_ISDIR(info.st_mode) && fcntl(fd, F_GETPATH, resolved) == 0;
    close(fd);
    return valid ? strdup(resolved) : NULL;
}

@interface LinkSendServicesProvider : NSObject
- (void)linksendSendFiles:(NSPasteboard *)pasteboard userData:(NSString *)userData error:(NSString **)error;
@end

static BOOL linksendIsTemporaryURL(NSURL *url) {
    NSString *path = url.URLByResolvingSymlinksInPath.path.stringByStandardizingPath;
    for (NSString *root in @[NSTemporaryDirectory(), @"/tmp", @"/private/tmp"]) {
        NSString *resolved = [NSURL fileURLWithPath:root].URLByResolvingSymlinksInPath.path.stringByStandardizingPath;
        if ([path isEqualToString:resolved] || [path hasPrefix:[resolved stringByAppendingString:@"/"]]) return YES;
    }
    return NO;
}

static BOOL linksendIsSandboxed(void) {
    SecTaskRef task = SecTaskCreateFromSelf(kCFAllocatorDefault);
    if (!task) return YES;
    CFTypeRef value = SecTaskCopyValueForEntitlement(task, CFSTR("com.apple.security.app-sandbox"), NULL);
    BOOL sandboxed = value && (CFGetTypeID(value) != CFBooleanGetTypeID() || CFBooleanGetValue(value));
    if (value) CFRelease(value);
    CFRelease(task);
    return sandboxed;
}

@implementation LinkSendServicesProvider
- (void)linksendSendFiles:(NSPasteboard *)pasteboard userData:(NSString *)userData error:(NSString **)error {
    (void)userData;
    NSArray<NSURL *> *urls = [pasteboard readObjectsForClasses:@[[NSURL class]]
        options:@{NSPasteboardURLReadingFileURLsOnlyKey: @YES}];
    if (urls.count == 0 || urls.count > 1024) {
        if (error) *error = @"请一次选择 1 至 1024 个文件或文件夹。";
        return;
    }
    NSMutableArray<NSString *> *paths = [NSMutableArray arrayWithCapacity:urls.count];
    NSString *failure = nil;
    for (NSURL *url in urls) {
        if (!url.isFileURL || url.path.length == 0 ||
            (url.host.length > 0 && ![url.host isEqualToString:@"localhost"])) {
            failure = @"仅支持用户选择的本地文件或已挂载文件夹。";
            break;
        }
        if ([url startAccessingSecurityScopedResource]) {
            [url stopAccessingSecurityScopedResource];
        }
        // A pasteboard URL can advertise security scope even in an unsandboxed
        // app. Do not mistake that return value for the file's access policy.
        // This adapter does not persist bookmarks, so sandboxed sources must
        // be selected again through a future durable-access implementation.
        if (linksendIsSandboxed()) {
            failure = @"所选文件依赖临时访问授权，请先复制到本地文件夹，再通过 Finder 选择。";
            break;
        }
        if (linksendIsTemporaryURL(url)) {
            failure = @"所选文件位于系统临时目录，请先复制到本地文件夹，再通过 Finder 选择。";
            break;
        }
        NSError *accessError = nil;
        if (![url checkResourceIsReachableAndReturnError:&accessError]) {
            failure = @"有文件已移动或暂时无法访问，请重新选择。";
            break;
        }
        int fd = open(url.fileSystemRepresentation, O_RDONLY | O_NONBLOCK | O_CLOEXEC);
        struct stat info;
        BOOL readable = fd >= 0 && fstat(fd, &info) == 0 && (S_ISREG(info.st_mode) || S_ISDIR(info.st_mode));
        if (fd >= 0) close(fd);
        if (!readable) {
            failure = @"有文件没有可持续的读取权限，请先复制到本地文件夹，再通过 Finder 选择。";
            break;
        }
        [paths addObject:url.path];
    }
    NSData *json = nil;
    if (!failure) {
        json = [NSJSONSerialization dataWithJSONObject:paths options:0 error:NULL];
        if (!json || json.length > 1024 * 1024) failure = @"所选路径总长度超出限制，请分批加入。";
    }
    if (!failure) {
        NSString *encoded = [[[NSString alloc] initWithData:json encoding:NSUTF8StringEncoding] autorelease];
        if (linksendReceiveFinderPaths((char *)encoded.UTF8String) != 0)
            failure = @"LinkSend 尚未就绪或未接受草稿，请打开应用后重试。";
    }
    if (failure) {
        if (error) *error = failure;
        return;
    }
}
@end

static LinkSendServicesProvider *linksendProvider = nil;

int linksendRegisterFinderServices(void) {
    if (![NSThread isMainThread] || !NSApp) return 1;
    if (linksendProvider) return 0;
    if (NSApp.servicesProvider != nil) return 2;
    linksendProvider = [[LinkSendServicesProvider alloc] init];
    [NSApp setServicesProvider:linksendProvider];
    NSUpdateDynamicServices();
    return 0;
}

void linksendUnregisterFinderServices(void) {
    if (![NSThread isMainThread]) return;
    if (NSApp.servicesProvider == linksendProvider) [NSApp setServicesProvider:nil];
    [linksendProvider release];
    linksendProvider = nil;
}
