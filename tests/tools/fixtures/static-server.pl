#!/usr/bin/env perl
use strict;
use warnings;
use IO::Socket::INET;
use POSIX ":sys_wait_h";

my $root = $ENV{FIXTURES_ROOT} || '/fixtures';
my $port = $ENV{FIXTURES_PORT} || 8080;

my $server = IO::Socket::INET->new(
    LocalAddr => '0.0.0.0',
    LocalPort => $port,
    Listen    => 64,
    ReuseAddr => 1,
) or die "listen on $port: $!";

$| = 1;
print "fixture server listening on $port\n";

# Reap forked children eagerly so they don't pile up as zombies. POSIX::SIGCHLD
# is delivered to the parent after fork; the default Perl handler does not
# auto-restart syscalls, so accept() can return undef with $!=EINTR and our
# loop would exit. Use a sigaction with SA_RESTART semantics by setting the
# handler before the loop and explicitly retrying accept on EINTR below.
$SIG{CHLD} = sub { local ($!, $?); while (waitpid(-1, WNOHANG) > 0) {} };
$SIG{PIPE} = 'IGNORE';

use Errno qw(EINTR);

while (1) {
    my $client = $server->accept;
    if (!$client) {
        next if $!{EINTR};
        last;
    }
    # Fork per connection so a slow/half-open client cannot block the loop.
    # The parent goes back to accept(); the child handles this one request
    # with a per-client wall-clock timeout via alarm().
    my $pid = fork();
    if (!defined $pid) {
        # fork() failed — handle in-process so the connection isn't dropped.
        handle_client($client);
        next;
    }
    if ($pid) {
        # Parent: close our copy of the client socket and accept more.
        close $client;
        next;
    }
    # Child:
    close $server;
    eval {
        local $SIG{ALRM} = sub { die "client_timeout\n" };
        alarm(10);
        handle_client($client);
        alarm(0);
    };
    exit 0;
}

sub handle_client {
    my ($client) = @_;
    $client->autoflush(1);
    my $request = <$client> || '';
    while (my $header = <$client>) {
        last if $header =~ /^\r?\n$/;
    }

    if ($request !~ m{^GET\s+([^ ]+)\s+HTTP/}i) {
        respond($client, 405, 'text/plain; charset=utf-8', 'method not allowed');
        return;
    }

    my $path = $1;
    $path =~ s/\?.*\z//;
    $path =~ s/%([0-9A-Fa-f]{2})/chr(hex($1))/eg;
    $path = '/index.html' if $path eq '/';

    if ($path eq '/redirect-to-internal') {
        redirect($client, 'http://127.0.0.1:9999/health');
        return;
    }

    if ($path =~ /\0/ || $path =~ m{(^|/)\.\.(/|\z)}) {
        respond($client, 403, 'text/plain; charset=utf-8', 'forbidden');
        return;
    }

    $path =~ s{^/+}{};
    my $file = "$root/$path";
    if (!-f $file) {
        respond($client, 404, 'text/plain; charset=utf-8', 'not found');
        return;
    }

    my $fh;
    if (!open $fh, '<:raw', $file) {
        respond($client, 500, 'text/plain; charset=utf-8', 'open failed');
        return;
    }
    local $/;
    my $body = <$fh>;
    close $fh;

    respond($client, 200, content_type($file), $body);
}

sub content_type {
    my ($file) = @_;
    return 'text/html; charset=utf-8' if $file =~ /\.html\z/;
    return 'text/plain; charset=utf-8' if $file =~ /\.txt\z/;
    return 'application/javascript; charset=utf-8' if $file =~ /\.js\z/;
    return 'text/css; charset=utf-8' if $file =~ /\.css\z/;
    return 'application/xml' if $file =~ /\.xml\z/;
    return 'application/gzip' if $file =~ /\.gz\z/;
    return 'application/octet-stream';
}

sub respond {
    my ($client, $status, $content_type, $body) = @_;
    my %reason = (
        200 => 'OK',
        302 => 'Found',
        403 => 'Forbidden',
        404 => 'Not Found',
        405 => 'Method Not Allowed',
        500 => 'Internal Server Error',
    );
    $body = '' if !defined $body;
    print {$client} "HTTP/1.1 $status " . ($reason{$status} || 'OK') . "\r\n";
    print {$client} "Content-Type: $content_type\r\n";
    print {$client} "Content-Length: " . length($body) . "\r\n";
    print {$client} "Connection: close\r\n\r\n";
    print {$client} $body;
    close $client;
}

sub redirect {
    my ($client, $location) = @_;
    print {$client} "HTTP/1.1 302 Found\r\n";
    print {$client} "Location: $location\r\n";
    print {$client} "Content-Length: 0\r\n";
    print {$client} "Connection: close\r\n\r\n";
    close $client;
}
