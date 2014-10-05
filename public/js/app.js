angular.module('dialog', ['ui.bootstrap']).
    factory('$dialog', function ($rootScope, $modal) {

        var module = {};

        // alert dialog
        module.alert = function (data) {
            if (!data) return false;
            var options = {
                backdrop: true,
                backdropClick: true,
                templateUrl: 'static/partials/alertDialogPartial.html',
                controller: 'AlertDialogController',
                resolve: {
                    args: function () {
                        return {
                            data: angular.copy(data)
                        }
                    }
                }
            };
            module.openDialog(options);
        };

        // generic dialog
        $rootScope.dialogs = [];
        module.openDialog = function (options) {
            if (!options) return false;

            $.each($rootScope.dialogs, function (i, dialog) {
                dialog.close();
            });
            $rootScope.dialogs = [];

            $modal.open(options);
        };

        return module;
    });

var panelLevels = {
    "OK"        : "panel-green",
    "INFO"      : "panel-primary",
    "WARNING"   : "panel-yellow",
    "CRITICAL"  : "panel-red",
    "ERROR"     : "panel-default"
}

var labelLevels = {
    "OK"        : "label-success",
    "INFO"      : "label-primary",
    "WARNING"   : "label-warning",
    "CRITICAL"  : "label-danger",
    "ERROR"     : "label-default"
}

var statusRowLevels = {
    "OK"        : "success",
    "INFO"      : "info",
    "WARNING"   : "warning",
    "CRITICAL"  : "danger",
    "ERROR"     : "active"
}

var logLevels = [
    "DEBUG",
    "OK",
    "INFO",
    "ERROR",
    "WARNING",
    "CRITICAL",
    "EMERGENCY"
]

var logRowLevels = {
    "DEBUG"     : "",
    "OK"        : "success",
    "INFO"      : "info",
    "ERROR"     : "active",
    "WARNING"   : "warning",
    "CRITICAL"  : "danger",
    "EMERGENCY" : "dancer"
}

angular.module('wigo-filters', [])
    .filter('getLevel', function() {
        return function(status){
            console.log(status,getLevel(status));
            return getLevel(status);
        };
    })
    .filter('panelLevelCssFilter', function() {
        return function(level) {
            return panelLevels[level];
        };
    })
    .filter('labelLevelCssFilter', function() {
            return function(level) {
                return labelLevels[level];
            };
        })
    .filter('statusTableRowCssFilter', function() {
            return function(status) {
                return statusRowLevels[getLevel(status)];
            };
    })
    .filter('logLevelTableRowCssFilter', function() {
        return function(status) {
            return logRowLevels[logLevels[status - 1]];
        };
    })
    .filter('logLevelFilter', function() {
        return function(logs,minLevel){
            min = logLevels.indexOf(minLevel);
            return _.filter(logs, function(log){
                return log.Level >= min + 1;
            })
        };
    });

angular.module('wigo', ['ngRoute', 'dialog', 'restangular', 'wigo-filters'])
	.config(function($routeProvider) {
		$routeProvider
			.when('/',      { controller: HostsCtrl,    templateUrl:'partials/hosts.html',  reloadOnSearch: false })
			.when('/group', { controller: GroupCtrl,    templateUrl:'partials/group.html',  reloadOnSearch: false })
			.when('/host',  { controller: HostCtrl,     templateUrl:'partials/host.html',   reloadOnSearch: false })
			.when('/logs',  { controller: LogsCtrl,     templateUrl:'partials/logs.html',   reloadOnSearch: false })
			.otherwise({ redirectTo: '/' });
    })
    .config(function(RestangularProvider) {
        RestangularProvider.setBaseUrl('/api');
    });

function getLevel(status) {
    if ( status < 100 ) {
        return "ERROR";
    } else if (status == 100) {
        return "OK";
    } else if (status < 200) {
        return "INFO";
    } else if (status < 300) {
        return "WARNING";
    } else if (status <500) {
        return "CRITICAL";
    } else {
        return "ERROR";
    }
}

function HostsCtrl($scope, Restangular, $dialog, $route, $location, $anchorScroll) {

    $scope.init = function() {
        $scope.load();
    }

    $scope.groups = [];
    $scope.load = function() {
        $scope.counts = {
            "OK" : 0,
            "INFO" : 0,
            "WARNING" : 0,
            "CRITICAL" : 0,
            "ERROR" : 0
        };
        Restangular.all('groups').getList().then(function(groups) {
            _.each(groups,function(group_name){
                Restangular.one('groups',group_name).get().then(function(group){
                    group.counts = {
                        "OK" : 0,
                        "INFO" : 0,
                        "WARNING" : 0,
                        "CRITICAL" : 0,
                        "ERROR" : 0
                    };
                    _.each(group.Hosts,function(host){
                        $scope.counts[getLevel(host.Status)]++;
                        group.counts[getLevel(host.Status)]++;
                    });
                    $scope.groups.push(group);
                });
            });
        });
    }

    $scope.goto = function(anchor){
        $location.hash(anchor);
        $anchorScroll();
    }

    $scope.gotoGroup = function(group){
         $location.search('name',group);
         $location.path('group');
         $location.hash();
         $route.reload();
    }

    $scope.init();
}

function GroupCtrl($scope, Restangular, $dialog, $route, $location, $anchorScroll) {

    $scope.init = function() {
        $scope.name = $location.search().name;
        $scope.load();
    }

    $scope.hosts = [];
    $scope.load = function() {
        if (!$scope.name) return;
        $scope.counts = {
            "OK" : 0,
            "INFO" : 0,
            "WARNING" : 0,
            "CRITICAL" : 0,
            "ERROR" : 0
        };
        Restangular.one('groups',$scope.name).get().then(function(group) {
            _.each(group.Hosts,function(host){
                host.counts = {
                    "OK" : 0,
                    "INFO" : 0,
                    "WARNING" : 0,
                    "CRITICAL" : 0,
                    "ERROR" : 0
                };
                _.each(host.Probes,function(probe){
                    $scope.counts[getLevel(probe.Status)]++;
                    host.counts[getLevel(probe.Status)]++;
                });
                $scope.hosts.push(host);
            });
        });
    }

    $scope.goto = function(anchor){
        $location.hash(anchor);
        $anchorScroll();
    }

    $scope.init();
}

function HostCtrl($scope, $dialog, $route, $location) {
    $scope.hello = "world";
}

function LogsCtrl($scope, Restangular, $dialog, $route, $location) {
    $scope.logLevels = logLevels;

    $scope.menu = {
        level : "OK"
    };

    $scope.load = function() {
        $scope.menu.group_select = "";
        $scope.menu.host_select = "";
        $scope.menu.probe_select = "";

        var _logs = Restangular;
        if($scope.menu.host){
            _logs = _logs.one('hosts',  $scope.menu.host)
        } else if($scope.menu.group){
            _logs = _logs.one('groups', $scope.menu.group)
        }
        if($scope.menu.probe){
            _logs = _logs.one('probes', $scope.menu.probe)
        }
        _logs = _logs.all('logs');

        _logs.getList().then(function(logs) {
            $scope.logs = logs;
        });

        var _indexes = Restangular.all('logs').one('indexes')
        _indexes.get().then(function(indexes){
            $scope.indexes = indexes;
        });
    }

    $scope.init = function() {
        $scope.menu.group    = $location.search().group;
        $scope.menu.host     = $location.search().host;
        $scope.menu.probe    = $location.search().probe;
        $scope.load();
    }

    $scope.updateUrl = function(){
        $location.search({
            group : $scope.menu.group,
            host : $scope.menu.host,
            probe : $scope.menu.probe,
        });
    }

    $scope.remove_group = function() {
        delete $scope.menu.group;
        $scope.load();
        $scope.updateUrl();
    }

    $scope.remove_host = function() {
        delete $scope.menu.host;
        $scope.load();
        $scope.updateUrl();
    }

    $scope.remove_probe = function() {
        delete $scope.menu.probe;
        $scope.load();
        $scope.updateUrl();
    }

    $scope.set_group = function(group) {
        $scope.menu.group = group;
        $scope.load();
        $scope.updateUrl();
    }

    $scope.set_host = function(host) {
        $scope.menu.host = host;
        $scope.load();
        $scope.updateUrl();
    }

    $scope.set_probe = function(probe) {
        $scope.menu.probe = probe;
        $scope.load();
        $scope.updateUrl();
    }

    $scope.init();
}

function AlertDialogController($rootScope, $scope, $modalInstance, args) {
    $rootScope.dialogs.push($scope);

    $scope.title = 'Success !';
    if (args.data.status != 100) $scope.title = 'Oops !';

    $scope.data = args.data;

    $scope.close = function (result) {
        $rootScope.dialogs = _.without($rootScope.dialogs, $scope);
        $modalInstance.close(result);
        if(args.callback) {
            args.callback(result);
        }
    };
}
