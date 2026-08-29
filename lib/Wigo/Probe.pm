package Wigo::Probe;

use strict;
use warnings;

use Getopt::Long;
use JSON;
use File::Basename;

require Exporter;
our @ISA = qw/Exporter/;
our @EXPORT_OK = qw/init config args result version status value message metrics add_metric detail raise persist output debug/;
our %EXPORT_TAGS = ( all => \@EXPORT_OK );

###
# VARS
###

my $CONFIG_PATH     = $ENV{"WIGO_PROBE_CONFIG_ROOT"} || "/etc/wigo/conf.d";
my $PERSIST_PATH    = "/tmp";

my $version    = "0.10";

my  $name       = basename($0);
my  $config     = {};
my  $args       = [];
my  $persist    = undef;

my  $result     =  {
    Version     => "0.10",

    Status      => 100,
    Value       => undef,
    Message     => "",

    Detail      => {},
    Metrics     => [],
};

###
# COMMAND LINE OPTIONS
###

my $opts = {};
GetOptions (
    $opts,
    'debug',
    '<>' => sub { push @$args, $_[0] }
) or die("Error in command line arguments\n");

my $json = JSON->new;
if ( exists $opts->{'debug'} )
{
    $json = JSON->new->pretty;
}

###
# DEBUG
###

sub debug {
    if ( exists $opts->{'debug'} )
    {
        print shift;
    }
}

###
# OUTPUT JSON
###

sub output {
    my $code = shift;

    if ( defined $result->{'Value'} )
    {
        $result->{'Value'} .= "";
    }
    else
    {
        if ( $result->{"Status"} == 100 )
        {
            $result->{'Value'} = 'OK';
        }
        elsif ( $result->{"Status"} > 100 and $result->{"Status"} < 199 )
        {
            $result->{'Value'} = 'INFO';
        }
        elsif ( $result->{"Status"} >= 200 and $result->{"Status"} < 300 )
        {
            $result->{'Value'} = 'WARN';
        }
        elsif ( $result->{"Status"} >= 300 and $result->{"Status"} < 500 )
        {
            $result->{'Value'} = 'CRIT';
        }
        else
        {
            $result->{'Value'} = 'ERROR';
        }
    }

    for my $metric ( @{$result->{'Metrics'}} )
    {
        defined $metric->{'Value'} and $metric->{'Value'} += 0;
    }

    print $json->encode( $result ) . "\n";

    if ( defined $code )
    {
        exit $code;
    }
}

###
# GETTER / SETTERS
###

sub init {
    my %params = @_;

    load_config($params{'config'});
    restore();
}

sub config
{
    return $config;
}

sub args
{
    return $args;
}

sub result
{
    return $result;
}

sub version
{
    if ( $@ )
    {
        $result->{"Version"} = shift;
    }
    else
    {
        return $result->{"Version"};
    }
}

sub status
{
    if ( @_ )
    {
        $result->{"Status"} = shift;
    }
    else
    {
        return $result->{"Status"};
    }
}

sub value
{
    if ( @_ )
    {
        $result->{"Value"} = shift;
    }
    else
    {
        return $result->{"Value"};
    }
}

sub message
{
    if ( @_ )
    {
        $result->{"Message"} = shift;
    }
    else
    {
        return $result->{"Message"};
    }
}

sub metrics
{
    if ( @_ )
    {
        $result->{"Metrics"} = shift;
    }
    else
    {
        return $result->{"Metrics"};
    }
}

sub add_metric
{
    push @{$result->{"Metrics"}}, shift;
}

sub detail
{
    if ( @_ )
    {
        $result->{"Detail"} = shift;
    }
    else
    {
        return $result->{"Detail"};
    }
}

sub persist
{
    if ( @_ )
    {
        $persist = shift;
        save();
    }
    else
    {
        return $persist;
    }
}

sub raise {
    my $status  = shift;

    result->{'Status'} = $status if result->{'Status'} < $status;
}

###
# CONFIG
###

sub save_config
{
    my $json = JSON->new->pretty;

    my $path = shift || $CONFIG_PATH . "/" . $name . ".conf";

    if ( open CONFIG, '>', $path )
        {
            eval {
                print CONFIG $json->encode($config)."\n";
            };
            close CONFIG;
    
            if ( $@ )
            {
                status 300;
                message "can't serialize config : $@";
                output 1;
            }
        }
        else
        {
            status 300;
            message "can't open config file $path for writing : $!";
            output 1;
        }
    
}

sub load_config
{
    my $defaults = shift || {};
    my $path     = $CONFIG_PATH . "/" . $name . ".conf";

    # The defaults first, whatever happens : a configuration file is an opinion
    # about some of the settings, not a replacement for all of them. Written the
    # other way round, adding one key to a file silently dropped every default
    # the probe relied on, and the probe then compared things against undef.
    $config = { %$defaults };

    if ( -r $path )
    {
        if ( ! open JSON_CONFIG, '<', $path )
        {
            status  500;
            message "Error while opening json config file for read : " . $!;
            output  1;
        }

        my $json;
        foreach my $line (<JSON_CONFIG>)
        {
            if ( $line =~ /^([^#;]*)([#;].*)?$/ )
            {
                $json .= $1;
            }
        }
        close JSON_CONFIG;

        my $fromFile;
        eval {
            $fromFile = decode_json( $json );
        };

        if ( $@ )
        {
            status  500;
            message "Error while decoding json config: " . $@;
            output  1;
        }

        if ( ref $fromFile eq "HASH" )
        {
            $config = { %$defaults, %$fromFile };
        }
        elsif ( defined $fromFile )
        {
            # Not a json object : nothing to merge, and pretending otherwise
            # would hide the mistake behind the defaults.
            $config = $fromFile;
        }

        if ( ref $config eq "HASH" and JSON::is_bool($config->{'enabled'}) and ! $config->{'enabled'} )
        {
            message "Probe is disabled";
            output  12;
        }
    }
}

###
# SAVE / LOAD PERSISTANT DATA
###

sub save
{
    return unless $persist;

    my $path = $PERSIST_PATH . "/" . $name . ".wigo";

    if ( open PERSIST, '>', $path )
    {
        eval {
            print PERSIST $json->encode($persist)."\n";
        };
        close PERSIST;

        if ( $@ )
        {
            status 300;
            message "can't serialize persistant data : $@";
            output 1;
        }
    }
    else
    {
        status 300;
        message "can't open persistant data file $path for writing : $!";
        output 1;
    }
}

sub restore
{
    my $path = $PERSIST_PATH . "/" . $name . ".wigo";

    return unless -e $path;

    if ( open PERSIST, '<', $path )
    {
        my @lines  = <PERSIST>;
        close PERSIST;

        chomp @lines;
        my $str = join "\n", @lines;
        return unless $str;

        eval {
            $persist = $json->decode( $str );
        };

        if ( $@ )
        {
            status 300;
            message "can't deserialize persistant data : $@";
            output 1;
        }
    }
    else
    {
        status 300;
        message "can't open persistant data file $path for reading : $!";
        output 1;
    }
}

1;
