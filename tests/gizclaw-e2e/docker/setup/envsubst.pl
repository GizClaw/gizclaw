#!/usr/bin/env perl
use strict;
use warnings;

my %allowed;
for my $argument (@ARGV) {
    while ($argument =~ /\$\{([A-Za-z_][A-Za-z0-9_]*)\}/g) {
        $allowed{$1} = 1;
    }
}

while (my $line = <STDIN>) {
    $line =~ s/\$\{([A-Za-z_][A-Za-z0-9_]*)\}/
        $allowed{$1} ? (defined $ENV{$1} ? $ENV{$1} : q{}) : $&
    /gex;
    print $line;
}
